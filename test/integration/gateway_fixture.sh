#!/usr/bin/env bash
# shellcheck disable=SC2154 # Globals are owned by the sourcing integration scenario.

# The integration binary resolves one exact development Gateway tag. Keep the
# run-local TLS wrapper temporary without destroying or retaining a
# contributor-owned image that already uses that tag.
gateway_fixture_snapshot_tag() {
  gateway_previous_image_id=$(
    docker image inspect --format '{{.Id}}' "$gateway_dev_tag" 2>/dev/null || true
  )
}

gateway_fixture_publish_tag() {
  local current_image_id

  gateway_fixture_image_id=$(
    docker image inspect --format '{{.Id}}' "$gateway_fixture_image"
  )
  [[ -n $gateway_fixture_image_id ]] || {
    echo "integration Gateway fixture image has no identity" >&2
    return 1
  }
  [[ $gateway_fixture_image_id != "$gateway_previous_image_id" ]] || {
    echo "integration Gateway fixture did not produce a fresh image" >&2
    return 1
  }

  current_image_id=$(
    docker image inspect --format '{{.Id}}' "$gateway_dev_tag" 2>/dev/null || true
  )
  [[ $current_image_id == "$gateway_previous_image_id" ]] || {
    echo "integration Gateway resolver tag changed before publication" >&2
    return 1
  }

  # Mark ownership before the only shared-tag mutation. An EXIT/INT/TERM trap
  # can then restore the predecessor even if interruption follows immediately.
  gateway_fixture_tag_installed=true
  docker image tag "$gateway_fixture_image_id" "$gateway_dev_tag"
  current_image_id=$(docker image inspect --format '{{.Id}}' "$gateway_dev_tag")
  [[ $current_image_id == "$gateway_fixture_image_id" ]] || {
    echo "integration Gateway resolver tag did not select the TLS fixture" >&2
    return 1
  }
}

gateway_fixture_restore_tag() {
  local current_image_id
  local restore_status=0

  if [[ $gateway_fixture_tag_installed == true ]]; then
    current_image_id=$(
      docker image inspect --format '{{.Id}}' "$gateway_dev_tag" 2>/dev/null || true
    )
    if [[ $current_image_id == "$gateway_fixture_image_id" ]]; then
      if [[ -n $gateway_previous_image_id ]]; then
        docker image tag "$gateway_previous_image_id" "$gateway_dev_tag" || restore_status=1
        current_image_id=$(
          docker image inspect --format '{{.Id}}' "$gateway_dev_tag" 2>/dev/null || true
        )
        [[ $current_image_id == "$gateway_previous_image_id" ]] || restore_status=1
      else
        docker image rm "$gateway_dev_tag" >/dev/null 2>&1 || restore_status=1
      fi
    elif [[ $current_image_id != "$gateway_previous_image_id" ]]; then
      # A concurrent writer owns this unexpected value. Never overwrite it or
      # pretend the contributor's predecessor was restored.
      echo "integration Gateway resolver tag changed before restoration" >&2
      restore_status=1
    fi
  fi

  docker image rm "$gateway_fixture_image" >/dev/null 2>&1 || true
  if ((restore_status == 0)); then
    gateway_fixture_tag_installed=false
  fi
  return "$restore_status"
}
