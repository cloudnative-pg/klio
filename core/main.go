/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/cloudnative-pg/klio/core/cmd"
	"github.com/cloudnative-pg/klio/core/internal/opentelemetry"
)

func main() {
	cobra.EnableTraverseRunHooks = true

	ctx := context.Background()

	shutdown := opentelemetry.Init(ctx)
	defer shutdown()

	cmd.Execute(ctx)
}
