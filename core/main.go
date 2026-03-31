/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/cloudnative-pg/klio/core/cmd"
)

func main() {
	cobra.EnableTraverseRunHooks = true

	ctx := context.Background()

	cmd.Execute(ctx)
}
