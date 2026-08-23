#!/usr/bin/env bash

# Resolve cleanup evidence without consuming or reconstructing the public
# Runtime image projection. The caller supplies fail() and the two output
# variables; exact build labels remain a trusted integration-only seam.
capture_runtime_image_for_cleanup() {
  local expected_runtime_id=$1
  local expected_source_digest=$2
  local records repository tag image_id extra evidence observed_id observed_owner observed_component observed_runtime_id observed_revision
  [[ $expected_runtime_id =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]] ||
    fail "managed Runtime ID is invalid for private cleanup discovery"
  [[ $expected_source_digest =~ ^sha256:[0-9a-f]{64}$ ]] ||
    fail "managed Runtime source digest is invalid for private cleanup discovery"
  records=$(docker image ls --all --no-trunc \
    --filter label=io.tobari.owner=default \
    --filter label=io.tobari.component=runtime-revision \
    --filter "label=io.tobari.runtime-id=$expected_runtime_id" \
    --filter "label=io.tobari.runtime-revision=$expected_source_digest" \
    --format '{{.Repository}}{{"\t"}}{{.Tag}}{{"\t"}}{{.ID}}' | sort -u)
  [[ -n $records && $(printf '%s\n' "$records" | wc -l | tr -d ' ') == 1 ]] ||
    fail "managed Runtime cleanup discovery did not return exactly one owned image tag"
  IFS=$'\t' read -r repository tag image_id extra <<<"$records"
  [[ -n $repository && $repository != '<none>' && -n $tag && $tag != '<none>' && -z $extra && $image_id =~ ^sha256:[0-9a-f]{64}$ ]] ||
    fail "managed Runtime cleanup discovery returned malformed image evidence"
  evidence=$(docker image inspect --format \
    '{{.Id}}{{"\t"}}{{index .Config.Labels "io.tobari.owner"}}{{"\t"}}{{index .Config.Labels "io.tobari.component"}}{{"\t"}}{{index .Config.Labels "io.tobari.runtime-id"}}{{"\t"}}{{index .Config.Labels "io.tobari.runtime-revision"}}' \
    "$repository:$tag")
  IFS=$'\t' read -r observed_id observed_owner observed_component observed_runtime_id observed_revision extra <<<"$evidence"
  [[ -z $extra && $observed_id == "$image_id" && $observed_owner == default &&
    $observed_component == runtime-revision && $observed_runtime_id == "$expected_runtime_id" &&
    $observed_revision == "$expected_source_digest" ]] ||
    fail "managed Runtime cleanup discovery did not revalidate exact ownership"
  runtime_image="$repository:$tag"
  runtime_image_id=$image_id
}
