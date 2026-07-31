/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package walserver

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudnative-pg/klio/core/internal/grpc"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
	"github.com/cloudnative-pg/klio/core/internal/repository"
)

// metadataFileName is the name of the file containing the
// cluster metadata.
const metadataFileName = "metadata"

// Get implements the relative GRPC call.
func (w *Implementation) Get(req *grpc.GetRequest, res grpc.WAL_GetServer) error {
	startTime := time.Now()

	err := w.getWAL(req, res)

	// Tag with the tier this server actually serves (tier-1 or tier-2), not a
	// hardcoded tier: the same Get implementation runs in both WAL servers.
	// Concat instead of append to avoid mutating the shared Attributes slice.
	attrs := slices.Concat(
		w.metrics.Attributes,
		[]attribute.KeyValue{opentelemetry.AttributeKeyClusterName.Of(req.GetClusterName())},
	)
	opentelemetry.RecordDuration(res.Context(), opentelemetry.ServerWal.GetDuration, time.Since(startTime), err, attrs...)

	return err
}

func (w *Implementation) getWAL(req *grpc.GetRequest, res grpc.WAL_GetServer) error { //nolint:cyclop
	logger := log.FromContext(res.Context())
	if err := repository.ValidatePathComponent(req.GetClusterName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid cluster name: %v", err.Error())
	}

	if err := repository.ValidatePathComponent(req.GetWalName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid WAL name: %v", err.Error())
	}

	if err := repository.ValidateWalFileName(req.GetWalName()); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid WAL name: %q", req.GetWalName())
	}

	// get_wal is a leaf span: the per-block read/send spans were removed in
	// favor of the WAL duration histograms, so the span context is not
	// propagated to any child.
	_, span := tracer.Start(
		res.Context(),
		opentelemetry.GetWalSpan,
		trace.WithAttributes(
			opentelemetry.AttributeKeyClusterName.Of(req.GetClusterName()),
			opentelemetry.AttributeKeyWalName.Of(req.GetWalName()),
		),
	)
	defer span.End()

	walReader, err := repository.NewReader(w.conn, req.GetClusterName(), req.GetWalName(), w.metrics)
	if errors.Is(err, os.ErrNotExist) {
		span.RecordError(fmt.Errorf("WAL not found: %v/%v", req.GetClusterName(), req.GetWalName()))
		return status.Errorf(codes.NotFound, "WAL not found: %v/%v", req.GetClusterName(), req.GetWalName())
	}
	if err != nil {
		return status.Errorf(codes.Internal, "error while reading WAL (opening): %v", err.Error())
	}

	for {
		readBytes, readError := walReader.ReadBlock(res.Context())
		if readError != nil && !errors.Is(readError, io.EOF) {
			return status.Errorf(codes.Internal, "error while reading WAL (reading into buffer): %v", readError.Error())
		}

		sendStart := time.Now()
		sendErr := res.Send(&grpc.GetResult{WalBlock: readBytes, SegmentSize: walReader.GetFileLength()})
		// Skip the terminal empty block sent on EOF: it carries no WAL data.
		if len(readBytes) > 0 {
			sendOutcome := opentelemetry.OutcomeSuccess
			if sendErr != nil {
				sendOutcome = opentelemetry.OutcomeFailure
			}
			w.metrics.RecordBlockStage(res.Context(), req.GetClusterName(), opentelemetry.PathGet, opentelemetry.StageSend,
				time.Since(sendStart), sendOutcome)
		}
		if sendErr != nil {
			return status.Errorf(codes.Internal, "error while writing WAL block (sending to client GRPC): %v",
				sendErr.Error())
		}

		if errors.Is(readError, io.EOF) {
			logger.Debug("WAL read completed", "name", req.GetWalName(), "cluster", req.GetClusterName())
			break
		}
	}

	return nil
}
