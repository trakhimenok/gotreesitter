#!/usr/bin/env bash
set -euo pipefail

# Seed a pinned public Java corpus for parser stress testing.
#
# The corpus stays outside this repository.  The checkout is sparse and
# materializes only .java files so perf runs do not pay for unrelated assets.
#
# Examples:
#   cgo_harness/seed_java_corpus.sh
#   cgo_harness/seed_java_corpus.sh --dest /tmp/gotreesitter-java-corpus/lucene

DEST_DIR="/tmp/gotreesitter-java-corpus/apache-lucene"
REPO_URL="https://github.com/apache/lucene.git"
REF="de92f115a3125624bf7c7141b8e700efa9a89427"

usage() {
  cat <<'EOF'
Usage: seed_java_corpus.sh [options]

Options:
  --dest DIR      Destination checkout directory
                  (default: /tmp/gotreesitter-java-corpus/apache-lucene)
  --ref SHA       Lucene commit to check out
                  (default: de92f115a3125624bf7c7141b8e700efa9a89427)
  -h, --help      Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dest)
      DEST_DIR="$2"
      shift 2
      ;;
    --ref)
      REF="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

DEST_DIR="${DEST_DIR/#\~/$HOME}"
mkdir -p "$(dirname "$DEST_DIR")"

if [[ ! -d "$DEST_DIR/.git" ]]; then
  rm -rf "$DEST_DIR"
  git clone --filter=blob:none --sparse --no-checkout "$REPO_URL" "$DEST_DIR"
fi

git -C "$DEST_DIR" fetch --depth=1 origin "$REF"
git -C "$DEST_DIR" sparse-checkout set --no-cone '/**/*.java'
git -C "$DEST_DIR" checkout --detach FETCH_HEAD

read -r files bytes < <(
  find "$DEST_DIR" -path '*/.git' -prune -o -name '*.java' -type f -printf '%s\n' |
    awk '{ count++; bytes += $1 } END { printf "%d %d\n", count, bytes }'
)

echo "java corpus: $DEST_DIR"
echo "ref:         $(git -C "$DEST_DIR" rev-parse HEAD)"
echo "files:       $files"
echo "bytes:       $bytes"
echo "usage:"
echo "  cd cgo_harness && GOT_JAVA_CORPUS_ROOT=$DEST_DIR go test . -tags treesitter_c_bench -run '^TestJavaCorpusTimeoutSweep$' -count=1 -v"
