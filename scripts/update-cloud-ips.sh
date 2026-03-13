#!/usr/bin/env bash
# Fetches cloud provider IP ranges and writes them to the embedded data file.
# Run this periodically to keep the blocklist current.
#
# Usage: ./scripts/update-cloud-ips.sh
#
# Requires: curl, jq

set -euo pipefail

OUTFILE="internal/plugin/sandbox/cloudips/data/ranges.json"
mkdir -p "$(dirname "$OUTFILE")"
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Fetching cloud provider IP ranges..."

# AWS
echo "  AWS..."
curl -sf "https://ip-ranges.amazonaws.com/ip-ranges.json" > "$TMPDIR/aws.json"
jq -r '[.prefixes[].ip_prefix] | unique' "$TMPDIR/aws.json" > "$TMPDIR/aws_v4.json"
jq -r '[.ipv6_prefixes[].ipv6_prefix] | unique' "$TMPDIR/aws.json" > "$TMPDIR/aws_v6.json"

# GCP
echo "  GCP..."
curl -sf "https://www.gstatic.com/ipranges/cloud.json" > "$TMPDIR/gcp.json"
jq -r '[.prefixes[] | select(.ipv4Prefix) | .ipv4Prefix] | unique' "$TMPDIR/gcp.json" > "$TMPDIR/gcp_v4.json"
jq -r '[.prefixes[] | select(.ipv6Prefix) | .ipv6Prefix] | unique' "$TMPDIR/gcp.json" > "$TMPDIR/gcp_v6.json"

# Oracle Cloud
echo "  Oracle Cloud..."
curl -sf "https://docs.oracle.com/en-us/iaas/tools/public_ip_ranges.json" > "$TMPDIR/oci.json"
jq -r '[.regions[].cidrs[].cidr] | unique' "$TMPDIR/oci.json" > "$TMPDIR/oci_v4.json"
echo '[]' > "$TMPDIR/oci_v6.json"

# Cloudflare (plain text, one CIDR per line)
echo "  Cloudflare..."
curl -sf "https://www.cloudflare.com/ips-v4" | jq -R -s 'split("\n") | map(select(. != ""))' > "$TMPDIR/cf_v4.json"
curl -sf "https://www.cloudflare.com/ips-v6" | jq -R -s 'split("\n") | map(select(. != ""))' > "$TMPDIR/cf_v6.json"

# Note: Azure is intentionally skipped because the download URL changes weekly
# and requires scraping a redirect page. Add it when we have a stable API.

# Assemble the final JSON.
TIMESTAMP=$(date -u '+%Y-%m-%dT%H:%M:%SZ')

jq -n \
  --arg ts "$TIMESTAMP" \
  --slurpfile aws_v4 "$TMPDIR/aws_v4.json" \
  --slurpfile aws_v6 "$TMPDIR/aws_v6.json" \
  --slurpfile gcp_v4 "$TMPDIR/gcp_v4.json" \
  --slurpfile gcp_v6 "$TMPDIR/gcp_v6.json" \
  --slurpfile oci_v4 "$TMPDIR/oci_v4.json" \
  --slurpfile oci_v6 "$TMPDIR/oci_v6.json" \
  --slurpfile cf_v4 "$TMPDIR/cf_v4.json" \
  --slurpfile cf_v6 "$TMPDIR/cf_v6.json" \
  '{
    lastUpdated: $ts,
    providers: {
      aws:        { ipv4: $aws_v4[0], ipv6: $aws_v6[0] },
      gcp:        { ipv4: $gcp_v4[0], ipv6: $gcp_v6[0] },
      oraclecloud: { ipv4: $oci_v4[0], ipv6: $oci_v6[0] },
      cloudflare: { ipv4: $cf_v4[0],  ipv6: $cf_v6[0] }
    }
  }' > "$OUTFILE"

echo "Written to $OUTFILE"
echo "Last updated: $TIMESTAMP"

# Show counts.
for provider in aws gcp oraclecloud cloudflare; do
  v4=$(jq -r ".providers.$provider.ipv4 | length" "$OUTFILE")
  v6=$(jq -r ".providers.$provider.ipv6 | length" "$OUTFILE")
  printf "  %-15s IPv4: %5s  IPv6: %5s\n" "$provider" "$v4" "$v6"
done
