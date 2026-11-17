#!/usr/bin/env python3
"""Export everything the importer understands from the Lemon Squeezy API into a ZIP archive.

    LEMONSQUEEZY_API_KEY=... python3 scripts/export-lemonsqueezy.py out.zip [--files] [--test-mode]

Only the Python standard library is used. The archive can be uploaded in the admin UI
(Migration -> Upload archive) or imported on the server, so the API key never has to leave your machine.
Respects the 300 requests/minute limit. Files are fetched immediately because their download URLs expire after 1 hour.
"""
import json, os, sys, time, urllib.error, urllib.request, zipfile

BASE = os.environ.get("LEMONSQUEEZY_API_BASE", "https://api.lemonsqueezy.com")
RESOURCES = ["stores", "customers", "products", "variants", "prices", "files", "orders", "order-items",
             "subscriptions", "subscription-items", "subscription-invoices", "license-keys", "license-key-instances"]


def get(url, key, retries=5):
    req = urllib.request.Request(url, headers={"Accept": "application/vnd.api+json", "Content-Type": "application/vnd.api+json",
                                               "Authorization": "Bearer " + key})
    for attempt in range(retries):
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                return json.load(r)
        except urllib.error.HTTPError as e:
            if e.code == 429 or e.code >= 500:
                wait = int(e.headers.get("Retry-After", "0") or 0) or 2 ** attempt
                print(f"  HTTP {e.code}, retrying in {wait}s", file=sys.stderr)
                time.sleep(wait)
                continue
            raise
    raise RuntimeError("giving up on " + url)


def main():
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    flags = {a for a in sys.argv[1:] if a.startswith("--")}
    key = os.environ.get("LEMONSQUEEZY_API_KEY", "")
    if not args or not key:
        sys.exit(__doc__)
    out = args[0]
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
        file_rows = []
        for res in RESOURCES:
            rows, url = [], f"{BASE}/v1/{res}?page%5Bsize%5D=100"
            while url:
                doc = get(url, key)
                rows.extend(doc.get("data", []))
                url = (doc.get("links") or {}).get("next")
                time.sleep(0.25)  # ~240 req/min, below the 300/min limit
            print(f"{res}: {len(rows)}")
            z.writestr(f"{res}.json", json.dumps({"jsonapi": {"version": "1.0"}, "data": rows}))
            if res == "files":
                file_rows = rows
        if "--files" in flags:
            for row in file_rows:
                attrs = row.get("attributes", {})
                ident, url = attrs.get("identifier"), attrs.get("download_url")
                if not ident or not url:
                    continue
                try:
                    with urllib.request.urlopen(url, timeout=600) as r:  # signed URL, no API key
                        z.writestr(f"files/{ident}", r.read())
                    print(f"file {ident} ({attrs.get('name')}) downloaded")
                except Exception as exc:  # keep going: the importer reports files that are missing
                    print(f"file {ident} FAILED: {exc}", file=sys.stderr)
    print("wrote", out)


if __name__ == "__main__":
    main()
