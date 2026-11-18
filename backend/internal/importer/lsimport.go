package importer

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"fulfillment/internal/db"
	"fulfillment/internal/service"
)

// normalizeResources expands the requested resources into the set of stages
// to run (empty = everything).
func normalizeResources(in []string) (map[string]bool, []string, error) {
	want := map[string]bool{}
	if len(in) == 0 {
		for _, r := range stageOrder {
			want[r] = true
		}
	}
	known := map[string]bool{}
	for _, r := range stageOrder {
		known[r] = true
	}
	for _, raw := range in {
		name := normalizeResourceName(raw)
		if name == "" {
			continue
		}
		if !known[name] {
			return nil, nil, fmt.Errorf("unknown resource %q (valid: %s)", raw, strings.Join(stageOrder, ", "))
		}
		if g, ok := resourceGroups[name]; ok {
			for _, m := range g {
				want[m] = true
			}
		} else {
			want[name] = true
		}
	}
	if len(want) == 0 {
		return nil, nil, fmt.Errorf("no resources selected")
	}
	var ordered []string
	for _, r := range stageOrder {
		if want[r] {
			ordered = append(ordered, r)
		}
	}
	return want, ordered, nil
}

// StartLemonSqueezyAPI pulls the store history from the Lemon Squeezy API.
func (r *Runner) StartLemonSqueezyAPI(createdBy string, o LSOptions) (*db.ImportJob, error) {
	if strings.TrimSpace(o.APIKey) == "" {
		return nil, service.Invalid("/api_key", "A Lemon Squeezy API key is required.")
	}
	want, ordered, err := normalizeResources(o.Resources)
	if err != nil {
		return nil, service.Invalid("/resources", err.Error())
	}
	onConflict, err := normalizeConflict(o.OnConflict)
	if err != nil {
		return nil, service.Invalid("/on_conflict", err.Error())
	}
	base := strings.TrimRight(strings.TrimSpace(o.BaseURL), "/")
	if base == "" {
		base = defaultLSBase
	}
	if u, err := url.Parse(base); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, service.Invalid("/base_url", "The Lemon Squeezy base URL must be an http(s) URL.")
	}
	storeIDs := append([]int64{}, o.StoreIDs...)
	opts := map[string]any{
		"api_key": redactKey(o.APIKey), "base_url": base, "test_mode": o.TestMode, "store_ids": storeIDs,
		"resources": ordered, "download_files": o.DownloadFiles, "dry_run": o.DryRun, "on_conflict": onConflict,
	}
	key := strings.TrimSpace(o.APIKey)
	return r.start(kindLSAPI, createdBy, opts, nil, func(ctx context.Context, j *jobCtl) error {
		client, err := newLSClient(r, base, key, j.info)
		if err != nil {
			return err
		}
		src := &apiSource{c: client, storeIDs: storeIDs, downloadFiles: o.DownloadFiles && !o.DryRun, httpc: r.client()}
		cfg := ingestCfg{testMode: o.TestMode, dryRun: o.DryRun, downloadFiles: o.DownloadFiles, storeIDs: storeIDs, resources: want, onConflict: onConflict}
		if o.DryRun {
			j.set("dry_run", true)
			j.info("dry run: everything is fetched and checked, nothing is written or downloaded")
		}
		return newIngest(r, j, cfg, src).run(ctx)
	})
}

// StartLemonSqueezyArchive imports a ZIP at zipPath; the runner owns the file and removes it when done.
func (r *Runner) StartLemonSqueezyArchive(createdBy, zipPath string, o ArchiveOptions) (*db.ImportJob, error) {
	cleanup := removeFile(zipPath)
	if strings.TrimSpace(zipPath) == "" {
		return nil, service.Invalid("/file", "An archive file is required.")
	}
	onConflict, err := normalizeConflict(o.OnConflict)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, service.Invalid("/on_conflict", err.Error())
	}
	opts := map[string]any{"source": "archive", "test_mode": o.TestMode, "download_files": o.DownloadFiles, "dry_run": o.DryRun, "on_conflict": onConflict}
	return r.start(kindLSFiles, createdBy, opts, cleanup, func(ctx context.Context, j *jobCtl) error {
		src, err := openArchive(zipPath, j, o.DownloadFiles && !o.DryRun, r.client())
		if err != nil {
			return err
		}
		defer src.Close()
		want, _, _ := normalizeResources(nil)
		cfg := ingestCfg{testMode: o.TestMode, dryRun: o.DryRun, downloadFiles: o.DownloadFiles, resources: want, onConflict: onConflict}
		if o.DryRun {
			j.set("dry_run", true)
			j.info("dry run: the archive is read and checked, nothing is written")
		}
		return newIngest(r, j, cfg, src).run(ctx)
	})
}
