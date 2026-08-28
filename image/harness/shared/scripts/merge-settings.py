#!/usr/bin/env python3
"""Deep-merge two settings.json files: merge-settings.py TARGET SOURCE OUT.

Used by both vendor setup scripts so a redeploy never clobbers local
customization (auth config, model overrides, extra permissions, ...):

  - dicts merge recursively; keys from SOURCE (the repo) win on conflict
  - lists of strings union, deduplicated, TARGET order first
  - other lists append SOURCE items not already present
  - scalars: SOURCE wins

Note: keys removed from SOURCE are NOT removed from TARGET — retire a
stale key by deleting it from the deployed file once by hand.
"""

import json
import sys


def merge(target, source):
    if isinstance(target, dict) and isinstance(source, dict):
        for key, value in source.items():
            target[key] = merge(target[key], value) if key in target else value
        return target
    if isinstance(target, list) and isinstance(source, list):
        if all(isinstance(x, str) for x in target + source):
            return list(dict.fromkeys(target + source))
        merged = list(target)
        for item in source:
            if item not in merged:
                merged.append(item)
        return merged
    return source


def main():
    if len(sys.argv) != 4:
        sys.exit(f"usage: {sys.argv[0]} TARGET SOURCE OUT")
    target_path, source_path, out_path = sys.argv[1:4]
    try:
        with open(target_path) as f:
            target = json.load(f)
    except (OSError, json.JSONDecodeError) as e:
        sys.exit(f"error: cannot read existing settings {target_path}: {e}")
    try:
        with open(source_path) as f:
            source = json.load(f)
    except (OSError, json.JSONDecodeError) as e:
        sys.exit(f"error: cannot read repo settings {source_path}: {e}")
    with open(out_path, "w") as f:
        json.dump(merge(target, source), f, indent=2, ensure_ascii=False)
        f.write("\n")


if __name__ == "__main__":
    main()
