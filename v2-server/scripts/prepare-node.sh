#!/usr/bin/env bash
set -euo pipefail

# Preparation is deliberately outside the demo hot path and never changes the source checkout.
source_commit=cab59a3c0ecd4aa31b939f29a5fca76d03a4e64e
# Keep the per-architecture digests in step with internal/nodeservices/opencode.go.
[[ $(uname -s) == Linux ]] || { echo 'Preparation supports Linux x86_64 and aarch64.' >&2; exit 1; }
case $(uname -m) in
  x86_64) target=x86_64-unknown-linux-gnu plugin_sha=88c00f81997fc7fe892f2d58a686f535678e562c7e000d33eb7ee3b1d0f754b5 ;;
  aarch64) target=aarch64-unknown-linux-gnu plugin_sha=41f58c82f343153f23771f7f314c22212145b3dcf039451b5a869dba24894547 ;;
  *) echo 'Preparation supports Linux x86_64 and aarch64.' >&2; exit 1 ;;
esac
plugin_url=https://github.com/ora-space/opencode-agent/releases/download/v0.6.11/ora-space.opencode-v0.6.11-$target.orax
: "${DESKTOP_REPOSITORY:?Set DESKTOP_REPOSITORY to a desktop repository containing the pinned commit}"
git -C "$DESKTOP_REPOSITORY" cat-file -e "$source_commit^{commit}"
assets_dir=${V2_NODE_ASSETS_DIR:-$HOME/.cache/ora-v2-node}
mkdir -p "$assets_dir"
assets_dir=$(cd "$assets_dir" && pwd -P)
export PATH="${CARGO_HOME:-$HOME/.cargo}/bin:$PATH"
deno_path=${V2_DENO_PATH:-$HOME/.deno/bin/deno}
[[ $("$deno_path" --version | head -1) == "deno 2.9.5 (stable, release, $target)" ]] || { echo "Deno 2.9.5 for $target is required." >&2; exit 1; }

source_dir="$assets_dir/source-$source_commit"
if [[ ! -f "$source_dir/.source-commit" ]]; then
  [[ ! -e "$source_dir" ]] || { echo "Unrecognized source directory: $source_dir" >&2; exit 1; }
  mkdir "$source_dir"
  git -C "$DESKTOP_REPOSITORY" archive "$source_commit" | tar -x -C "$source_dir"
  printf '%s\n' "$source_commit" > "$source_dir/.source-commit"
fi
[[ $(cat "$source_dir/.source-commit") == "$source_commit" ]] || { echo 'Source marker mismatch.' >&2; exit 1; }
(cd "$source_dir" && cargo +1.95.0 build --locked -p ora-node -p ora-process-host -p ora-process-guardian)
mkdir -p "$assets_dir/bin"
for binary in ora-node ora-process-host ora-process-guardian; do
  cp -- "$source_dir/target/debug/$binary" "$assets_dir/bin/$binary"
done
cp -- "$deno_path" "$assets_dir/bin/deno"
plugin="$assets_dir/opencode-0.6.11.orax"
if [[ ! -f "$plugin" ]]; then
  curl --fail --location --retry 3 --output "$plugin.download" "$plugin_url"
  printf '%s  %s\n' "$plugin_sha" "$plugin.download" | sha256sum --check --status
  mv -- "$plugin.download" "$plugin"
fi
printf '%s  %s\n' "$plugin_sha" "$plugin" | sha256sum --check --status

# Source revision and plugin digest are pinned above. Locally built executables and the trusted,
# version-checked Deno installation are hashed here and verified again by runtime startup.
python3 - "$assets_dir" "$source_commit" <<'PY'
import hashlib, json, pathlib, sys
root = pathlib.Path(sys.argv[1])
def asset(path):
    with path.open('rb') as source:
        digest = hashlib.file_digest(source, 'sha256').hexdigest()
    return {'Path': str(path), 'SHA256': digest}
manifest = {'version': 1, 'sourceCommit': sys.argv[2],
            'node': asset(root/'bin'/'ora-node'),
            'host': asset(root/'bin'/'ora-process-host'),
            'guardian': asset(root/'bin'/'ora-process-guardian'),
            'deno': asset(root/'bin'/'deno'),
            'plugin': asset(root/'opencode-0.6.11.orax')}
target = root/'assets.json'
target.write_text(json.dumps(manifest, indent=2) + '\n')
print(target)
PY
