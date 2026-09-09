---
sidebar_position: 91
---

# Upgrade Notes

This page lists version-specific changes that may require
manual action when upgrading Klio. For the upgrade procedure,
see the [Helm chart page](helm_chart.mdx#upgrades).

## Unreleased

### Klio-managed retention policies

Retention is now evaluated by the Klio server against its own backup catalog
instead of being delegated to Kopia.

- The `retention` block of the `PluginConfiguration` changed shape. The
  Kopia-style `keepLatest`, `keepHourly`, `keepDaily`, `keepWeekly`,
  `keepMonthly` and `keepAnnual` fields are replaced by a single `latest`
  field, which keeps the given number of most recent backups. Update any
  `tier1.retention` and `tier2.retention` blocks accordingly. Omit the block to
  keep every backup; when present, `latest` must be at least `1`.
- The `klio retention` command has been removed. Retention is configured only
  through the `PluginConfiguration`.
