package k8sapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"

	"github.com/cloudnative-pg/klio/core/internal/client/klioclient"
	"github.com/cloudnative-pg/klio/core/internal/client/klioclient/kopia"
	"github.com/cloudnative-pg/klio/core/internal/k8sapi/v1alpha1"
)

// REST is the API Server implementation for the KlioBackup resource.
type REST struct {
	connection klioclient.Client
}

var (
	_ rest.Storage              = &REST{}
	_ rest.Lister               = &REST{}
	_ rest.Getter               = &REST{}
	_ rest.Scoper               = &REST{}
	_ rest.SingularNameProvider = &REST{}
)

// NewREST creates a REST interface for the API Server Extension
// to use.
func NewREST(connection klioclient.Client) *REST {
	return &REST{
		connection: connection,
	}
}

// GetSingularName returns the singular name of resources. This is used by kubectl discovery to have a singular
// name representation of resources. In case of shortcut conflicts(with CRD shortcuts), a singular name should
// always map to this resource.
func (r *REST) GetSingularName() string {
	return "kliobackup"
}

// New returns an empty object that can be used with Create and Update after request data has been put into it.
// This object must be a pointer type for use with Codec.DecodeInto([]byte, runtime.Object).
func (r *REST) New() runtime.Object {
	return &v1alpha1.KlioBackup{}
}

// Destroy cleans up its resources on shutdown.
// Destroy has to be implemented in a thread-safe way and be prepared
// for being called more than once.
func (r *REST) Destroy() {
}

// Get finds a resource in the storage by name and returns it.
// Although it can return an arbitrary error value, IsNotFound(err) is true for the
// returned error value err when the specified resource is not found.
func (r *REST) Get(ctx context.Context, name string, _ *metav1.GetOptions) (runtime.Object, error) {
	contextLogger := log.FromContext(ctx)

	splittedName := strings.SplitN(name, ".", 2)
	if len(splittedName) != 2 {
		return nil, apierrors.NewNotFound(
			schema.GroupResource{
				Group:    "klio.cnpg.io",
				Resource: "kliobackups",
			},
			name,
		)
	}
	clusterName := splittedName[0]
	backupName := splittedName[1]

	metadata, err := r.connection.GetMetadata(ctx, clusterName, backupName)

	var nbf kopia.NoBackupFoundError
	if errors.As(err, &nbf) || metadata == nil {
		return nil, apierrors.NewNotFound(
			schema.GroupResource{
				Group:    "klio.cnpg.io",
				Resource: "kliobackups",
			},
			name,
		)
	}
	if err != nil {
		contextLogger.Error(err, "Error getting backup metadata")
		return nil, err
	}

	klioBackupObject := backupMetadataToKlioBackup(*metadata)

	return &klioBackupObject, nil
}

// NewList returns an empty object that can be used with the List call.
// This object must be a pointer type for use with Codec.DecodeInto([]byte, runtime.Object).
func (r *REST) NewList() runtime.Object {
	return &v1alpha1.KlioBackupList{}
}

// List selects resources in the storage which match to the selector. 'options' can be nil.
func (r *REST) List(ctx context.Context, _ *internalversion.ListOptions) (runtime.Object, error) {
	contextLogger := log.FromContext(ctx)

	backupMetadataList, err := r.connection.ListBackups(ctx, "")
	if err != nil {
		contextLogger.Error(err, "Error listing backups")
		return nil, err
	}

	result := v1alpha1.KlioBackupList{}
	result.Items = make([]v1alpha1.KlioBackup, len(backupMetadataList))

	for i := range backupMetadataList {
		result.Items[i] = backupMetadataToKlioBackup(backupMetadataList[i])
	}

	return &result, nil
}

// ConvertToTable converts an object list to a table representation
// to be used by kubectl.
func (r *REST) ConvertToTable(
	_ context.Context,
	object runtime.Object,
	opts runtime.Object,
) (*metav1.Table, error) {
	result := &metav1.Table{
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{
				Name: "Name",
				Type: "string",
			},
			{
				Name: "Cluster Name",
				Type: "string",
			},
			{
				Name: "Started At",
				Type: "string",
			},
			{
				Name: "Stopped At",
				Type: "string",
			},
		},
	}

	tableOptions, ok := opts.(*metav1.TableOptions)
	if !ok {
		return nil, apierrors.NewBadRequest("TableOptions is not of type *metav1.TableOptions")
	}

	var items []v1alpha1.KlioBackup
	switch t := object.(type) {
	case *v1alpha1.KlioBackup:
		items = append(items, *t)

	case *v1alpha1.KlioBackupList:
		items = object.(*v1alpha1.KlioBackupList).Items //nolint:forcetypeassert
	}

	for i := range items {
		row := metav1.TableRow{
			Cells: []any{
				items[i].Name,
				items[i].Spec.ClusterName,
				items[i].Status.StartedAt.String(),
				items[i].Status.StoppedAt.String(),
			},
		}

		if tableOptions.IncludeObject != metav1.IncludeNone {
			row.Object = runtime.RawExtension{Object: &items[i]}
		}
		result.Rows = append(result.Rows, row)
	}

	return result, nil
}

func backupMetadataToKlioBackup(metadata klioclient.BackupMetadata) v1alpha1.KlioBackup {
	tablespaces := make([]v1alpha1.TablespaceLayout, len(metadata.Tablespaces))
	for i := range metadata.Tablespaces {
		tablespaces[i].Name = metadata.Tablespaces[i].Name
		tablespaces[i].Oid = metadata.Tablespaces[i].Oid
		tablespaces[i].Path = metadata.Tablespaces[i].Path
		tablespaces[i].Annotations = metadata.Tablespaces[i].Annotations
	}

	return v1alpha1.KlioBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name: metadata.ClusterName + "." + metadata.Name,
		},
		Spec: v1alpha1.KlioBackupSpec{
			ClusterName: metadata.ClusterName,
			BackupID:    metadata.Name,
		},
		Status: v1alpha1.KlioBackupStatus{
			StartLSN:    metadata.StartLSN,
			EndLSN:      metadata.EndLSN,
			StartWAL:    metadata.StartWAL,
			EndWAL:      metadata.EndWAL,
			StartedAt:   metav1.NewTime(time.Unix(metadata.StartedAt, 0)),
			StoppedAt:   metav1.NewTime(time.Unix(metadata.StoppedAt, 0)),
			Annotations: metadata.Annotations,
			Tablespaces: tablespaces,
		},
	}
}

// NamespaceScoped returns true if the storage is namespaced.
func (r *REST) NamespaceScoped() bool {
	return false
}
