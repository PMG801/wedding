#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

smoke_env_file="$(mktemp "${TMPDIR:-/tmp}/wedding-smoke.XXXXXX")"
chmod 600 "$smoke_env_file"
cat >"$smoke_env_file" <<'EOF'
BODA_ADDR=:8080
BODA_DATA_DIR=/srv/evento
BODA_EVENT_TOKEN=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
BODA_ADMIN_PASSWORD_HASH='$argon2id$v=19$m=8,t=1,p=1$c21va2VzYWx0$Ko3zYasvKeqcgGGZmu4jyg'
BODA_SESSION_KEY=0123456789abcdef0123456789abcdef
BODA_DISK_MIN_FREE_BYTES=1
EOF
export BODA_ENV_FILE="$smoke_env_file"
export BODA_DOMAIN=:80
smoke_base_url=http://localhost:8081
smoke_event_token=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
smoke_photo_id=123e4567-e89b-42d3-a456-426614174000
smoke_photo_file="$(mktemp "${TMPDIR:-/tmp}/wedding-smoke-photo.XXXXXX")"
smoke_guest_headers="$(mktemp "${TMPDIR:-/tmp}/wedding-smoke-guest.XXXXXX")"
smoke_media_headers="$(mktemp "${TMPDIR:-/tmp}/wedding-smoke-media-headers.XXXXXX")"
smoke_media_body="$(mktemp "${TMPDIR:-/tmp}/wedding-smoke-media-body.XXXXXX")"
smoke_admin_headers="$(mktemp "${TMPDIR:-/tmp}/wedding-smoke-admin.XXXXXX")"

cleanup() {
  local status=$?
  trap - EXIT

  if (( status != 0 )); then
    echo "Smoke test failed; printing Docker Compose logs:" >&2
    docker compose logs || true
  fi

  docker compose down -v || {
    local cleanup_status=$?
    if (( status == 0 )); then
      status=$cleanup_status
    fi
  }
  rm -f -- "$smoke_env_file" "$smoke_photo_file" "$smoke_guest_headers" "$smoke_media_headers" "$smoke_media_body" "$smoke_admin_headers"
  exit "$status"
}
trap cleanup EXIT

docker compose up -d --build --wait

health_ok=false
for ((attempt = 1; attempt <= 30; attempt++)); do
  if health_response="$(curl -fsS "$smoke_base_url/api/health" 2>/dev/null)" && [[ "$health_response" == *'"status":"ok"'* ]]; then
    health_ok=true
    break
  fi
  if (( attempt < 30 )); then
    sleep 1
  fi
done
if [[ "$health_ok" != true ]]; then
  echo 'Health endpoint did not return a body containing "status":"ok" within 30 attempts.' >&2
  exit 1
fi

echo 'Health endpoint passed.'

homepage="$(curl -fsS "$smoke_base_url/")"
if [[ "$homepage" != *'<div id="app"'* ]]; then
  echo 'Homepage did not contain the frontend mount element <div id="app".' >&2
  exit 1
fi

echo 'Homepage mount element passed.'

guest_status="$(curl -sS -D "$smoke_guest_headers" -o /dev/null -w '%{http_code}' \
  "$smoke_base_url/e/$smoke_event_token")"
if [[ "$guest_status" != 303 ]]; then
  echo "Guest QR entry returned HTTP $guest_status; want 303." >&2
  exit 1
fi
guest_cookie="$(awk 'tolower($1) == "set-cookie:" && $2 ~ /^guest_session=/ { sub(/;.*/, "", $2); print $2; exit }' "$smoke_guest_headers")"
if [[ -z "$guest_cookie" ]]; then
  echo 'Guest QR entry did not set a guest session cookie.' >&2
  exit 1
fi
echo 'Guest QR entry passed.'

printf '%s' 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADUlEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC' |
  base64 --decode >"$smoke_photo_file"

unauthenticated_upload_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H 'Content-Type: image/png' --data-binary "@$smoke_photo_file" \
  "$smoke_base_url/api/media/$smoke_photo_id")"
if [[ "$unauthenticated_upload_status" != 401 ]]; then
  echo "Unauthenticated photo upload returned HTTP $unauthenticated_upload_status; want 401." >&2
  exit 1
fi

# Forward the cookie manually because the local smoke test uses HTTP and the cookie remains Secure.
upload_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H "Cookie: $guest_cookie" -H 'Content-Type: image/png' \
  --data-binary "@$smoke_photo_file" "$smoke_base_url/api/media/$smoke_photo_id")"
if [[ "$upload_status" != 201 ]]; then
  echo "Authenticated photo upload returned HTTP $upload_status; want 201." >&2
  exit 1
fi
echo 'Authenticated photo upload passed.'

retry_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H "Cookie: $guest_cookie" -H 'Content-Type: image/png' \
  --data-binary "@$smoke_photo_file" "$smoke_base_url/api/media/$smoke_photo_id")"
if [[ "$retry_status" != 200 ]]; then
  echo "Idempotent photo retry returned HTTP $retry_status; want 200." >&2
  exit 1
fi
echo 'Idempotent photo retry passed.'

unauthenticated_gallery_status="$(curl -sS -o /dev/null -w '%{http_code}' "$smoke_base_url/api/media")"
if [[ "$unauthenticated_gallery_status" != 401 ]]; then
  echo "Unauthenticated gallery returned HTTP $unauthenticated_gallery_status; want 401." >&2
  exit 1
fi

gallery_response="$(curl -fsS -H "Cookie: $guest_cookie" "$smoke_base_url/api/media")"
original_url="$(printf '%s' "$gallery_response" | grep -o '"original_url":"[^"]*"' | head -n 1 | cut -d '"' -f 4)"
if [[ "$gallery_response" != *"\"id\":\"$smoke_photo_id\""* || "$original_url" != /m/o/* ]]; then
  echo "Gallery did not list the uploaded photo with an original URL: $gallery_response" >&2
  exit 1
fi
echo 'Authenticated gallery listing passed.'

unauthenticated_media_status="$(curl -sS -o /dev/null -w '%{http_code}' "$smoke_base_url$original_url")"
if [[ "$unauthenticated_media_status" != 401 ]]; then
  echo "Unauthenticated media file returned HTTP $unauthenticated_media_status; want 401." >&2
  exit 1
fi

media_status="$(curl -sS -D "$smoke_media_headers" -o "$smoke_media_body" -w '%{http_code}' \
  -H "Cookie: $guest_cookie" -H 'Accept-Encoding: gzip, zstd' "$smoke_base_url$original_url")"
if [[ "$media_status" != 200 ]] || ! cmp -s "$smoke_photo_file" "$smoke_media_body"; then
  echo "Authenticated media file returned HTTP $media_status or bytes that differ from the upload." >&2
  exit 1
fi
if ! grep -qi '^cache-control: private, max-age=31536000, immutable' "$smoke_media_headers" ||
  ! grep -qi '^x-content-type-options: nosniff' "$smoke_media_headers" ||
  grep -qi '^content-encoding:' "$smoke_media_headers"; then
  echo 'Media file headers are missing immutable caching or nosniff, or the file was compressed.' >&2
  cat "$smoke_media_headers" >&2
  exit 1
fi
echo 'Authenticated media file passed.'

download_disposition="$(curl -sS -o /dev/null -D - -H "Cookie: $guest_cookie" "$smoke_base_url$original_url?dl=1" |
  tr -d '\r' | awk 'tolower($1) == "content-disposition:" { print $2 }')"
if [[ "$download_disposition" != attachment* ]]; then
  echo "Media download did not set Content-Disposition attachment (got '$download_disposition')." >&2
  exit 1
fi
echo 'Media download disposition passed.'

# Static app files (S13 of odd/tasks/mobile-ui-polish.md): hashed assets are cached for a year,
# the HTML shell is always revalidated, and a missing asset is a 404, never the HTML shell.
shell_cache="$(curl -sS -o /dev/null -D - "$smoke_base_url/" | tr -d '\r' |
  awk 'tolower($1) == "cache-control:" { $1 = ""; sub(/^ /, ""); print }')"
if [[ "$shell_cache" != 'no-cache' ]]; then
  echo "The HTML shell returned Cache-Control '$shell_cache'; want 'no-cache'." >&2
  exit 1
fi
asset_path="$(curl -fsS "$smoke_base_url/" | grep -o '/assets/index-[^"]*\.js' | head -n 1)"
asset_cache="$(curl -fsS -o /dev/null -D - "$smoke_base_url$asset_path" | tr -d '\r' |
  awk 'tolower($1) == "cache-control:" { $1 = ""; sub(/^ /, ""); print }')"
if [[ -z "$asset_path" || "$asset_cache" != 'public, max-age=31536000, immutable' ]]; then
  echo "Asset '$asset_path' returned Cache-Control '$asset_cache'; want immutable for a year." >&2
  exit 1
fi
missing_asset="$(curl -sS -o /dev/null -D - -w 'status=%{http_code}' "$smoke_base_url/assets/missing-0000.js" | tr -d '\r')"
if [[ "$missing_asset" != *'status=404'* ]] || grep -qi '^cache-control: public, max-age=31536000, immutable' <<<"$missing_asset"; then
  echo "A missing asset must be an uncached 404: $missing_asset" >&2
  exit 1
fi
echo 'Static asset caching passed.'

for path in /m/o/missing.jpg /m/d/missing_t.jpg /m/hidden/x.jpg /m/o/../data/boda.db; do
  missing_status="$(curl -sS --path-as-is -o /dev/null -w '%{http_code}' -H "Cookie: $guest_cookie" "$smoke_base_url$path")"
  if [[ "$missing_status" != 404 ]]; then
    echo "Media path $path returned HTTP $missing_status; want 404." >&2
    exit 1
  fi
done
echo 'Unpublished media paths passed.'

range_status="$(curl -sS -o /dev/null -w '%{http_code} %{size_download}' -H "Cookie: $guest_cookie" -H 'Range: bytes=0-3' \
  "$smoke_base_url$original_url")"
if [[ "$range_status" != '206 4' ]]; then
  echo "Media Range request returned '$range_status'; want '206 4'." >&2
  exit 1
fi
echo 'Media Range request passed.'

# A minimal HEIC signature is enough: the server checks the ftyp brand only.
smoke_heic_id=123e4567-e89b-42d3-a456-426614174001
printf '\000\000\000\030ftypheic\000\000\000\000mif1heic' >"$smoke_photo_file"
heic_upload_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H "Cookie: $guest_cookie" --data-binary "@$smoke_photo_file" "$smoke_base_url/api/media/$smoke_heic_id")"
if [[ "$heic_upload_status" != 201 ]]; then
  echo "HEIC upload returned HTTP $heic_upload_status; want 201." >&2
  exit 1
fi
heic_url="$(curl -fsS -H "Cookie: $guest_cookie" "$smoke_base_url/api/media?limit=1" | grep -o '"original_url":"[^"]*"' | cut -d '"' -f 4)"
heic_type="$(curl -sS -o /dev/null -w '%{content_type}' -H "Cookie: $guest_cookie" "$smoke_base_url$heic_url")"
if [[ "$heic_url" != *.heic || "$heic_type" != image/heic ]]; then
  echo "HEIC media '$heic_url' was served as '$heic_type'; want image/heic." >&2
  exit 1
fi
echo 'HEIC content type passed.'

# A synthetic MP4: the server checks the ftyp major brand only. 4 KiB of
# deterministic payload makes the Range and byte-identity checks meaningful.
smoke_video_id=123e4567-e89b-42d3-a456-426614174002
{ printf '\000\000\000\030ftypmp42\000\000\000\000mp42isom'; head -c 4096 /dev/zero | tr '\0' 'v'; } >"$smoke_photo_file"
video_upload_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H "Cookie: $guest_cookie" -H 'Content-Type: video/mp4' --data-binary "@$smoke_photo_file" "$smoke_base_url/api/media/$smoke_video_id")"
if [[ "$video_upload_status" != 201 ]]; then
  echo "Video upload returned HTTP $video_upload_status; want 201." >&2
  exit 1
fi
video_retry_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H "Cookie: $guest_cookie" -H 'Content-Type: video/mp4' --data-binary "@$smoke_photo_file" "$smoke_base_url/api/media/$smoke_video_id")"
if [[ "$video_retry_status" != 200 ]]; then
  echo "Video retry returned HTTP $video_retry_status; want 200." >&2
  exit 1
fi
video_item="$(curl -fsS -H "Cookie: $guest_cookie" "$smoke_base_url/api/media?limit=1")"
video_url="$(printf '%s' "$video_item" | grep -o '"original_url":"[^"]*"' | cut -d '"' -f 4)"
if [[ "$video_url" != *.mp4 || "$video_item" != *'"mime":"video/mp4"'* ]]; then
  echo "Gallery listed the video as '$video_item'; want a .mp4 original with mime video/mp4." >&2
  exit 1
fi
video_type="$(curl -sS -o "$smoke_media_body" -w '%{content_type}' -H "Cookie: $guest_cookie" "$smoke_base_url$video_url")"
if [[ "$video_type" != video/mp4 ]] || ! cmp -s "$smoke_photo_file" "$smoke_media_body"; then
  echo "Video media was served as '$video_type' or differs from the upload." >&2
  exit 1
fi
video_range="$(curl -sS -o /dev/null -D - -H "Cookie: $guest_cookie" -H 'Range: bytes=100-1099' "$smoke_base_url$video_url" | tr -d '\r')"
if [[ "$video_range" != *' 206'* || "${video_range,,}" != *'content-range: bytes 100-1099/'* ]]; then
  echo "Video Range request returned: $video_range" >&2
  exit 1
fi
echo 'Video upload, playback type and Range passed.'

# Moderation: the smoke-only admin password matches BODA_ADMIN_PASSWORD_HASH above.
admin_status="$(curl -sS -D "$smoke_admin_headers" -o /dev/null -w '%{http_code}' \
  -X POST -H 'Content-Type: application/json' --data '{"password":"smoke-admin"}' "$smoke_base_url/api/admin/login")"
admin_cookie="$(awk 'tolower($1) == "set-cookie:" && $2 ~ /^admin_session=/ { sub(/;.*/, "", $2); print $2; exit }' "$smoke_admin_headers")"
if [[ "$admin_status" != 204 || -z "$admin_cookie" ]]; then
  echo "Admin login returned HTTP $admin_status without an admin cookie; want 204 and a cookie." >&2
  exit 1
fi
moderate() {
  curl -sS -o /dev/null -w '%{http_code}' -X PATCH -H "Cookie: $admin_cookie" -H 'Content-Type: application/json' \
    --data "{\"visibility\":\"$1\"}" "$smoke_base_url/api/admin/media/$smoke_photo_id"
}
media_status() {
  curl -sS -o /dev/null -w '%{http_code}' -H "Cookie: $guest_cookie" "$smoke_base_url$original_url"
}
if [[ "$(moderate hidden)" != 200 || "$(media_status)" != 404 ]]; then
  echo 'Hiding the photo did not make its original URL return 404.' >&2
  exit 1
fi
if [[ "$(curl -fsS -H "Cookie: $guest_cookie" "$smoke_base_url/api/media")" == *"\"id\":\"$smoke_photo_id\""* ]]; then
  echo 'The gallery still lists the hidden photo.' >&2
  exit 1
fi
if [[ "$(curl -fsS -H "Cookie: $admin_cookie" "$smoke_base_url/api/admin/media?visibility=hidden")" != *"\"id\":\"$smoke_photo_id\""* ]]; then
  echo 'The admin hidden listing does not include the hidden photo.' >&2
  exit 1
fi
delete_status="$(curl -sS -o /dev/null -w '%{http_code}' -X DELETE -H "Cookie: $admin_cookie" "$smoke_base_url/api/admin/media/$smoke_photo_id")"
if [[ "$delete_status" != 405 ]]; then
  echo "DELETE on media returned HTTP $delete_status; want 405 (media is never deleted)." >&2
  exit 1
fi
if [[ "$(moderate visible)" != 200 || "$(media_status)" != 200 ]]; then
  echo 'Showing the photo again did not restore its original URL.' >&2
  exit 1
fi
echo 'Admin moderation hide, hidden listing, no delete and show passed.'

# Likes: one like per guest cookie, idempotent, a second guest counts apart, and counts survive a restart.
like() {
  curl -sS -w ' %{http_code}' -X "$1" -H "Cookie: $2" "$smoke_base_url/api/media/$smoke_photo_id/like"
}
second_guest_cookie="$(curl -sS -o /dev/null -D - "$smoke_base_url/e/$smoke_event_token" |
  awk 'tolower($1) == "set-cookie:" && $2 ~ /^guest_session=/ { sub(/;.*/, "", $2); print $2; exit }')"
if [[ -z "$second_guest_cookie" ]]; then
  echo 'A second guest QR entry did not set a guest session cookie.' >&2
  exit 1
fi
first_like="$(like PUT "$guest_cookie")"
repeated_like="$(like PUT "$guest_cookie")"
second_like="$(like PUT "$second_guest_cookie")"
if [[ "$first_like" != *'"like_count":1,"liked_by_me":true}'*' 200' ||
  "$repeated_like" != *'"like_count":1,"liked_by_me":true}'*' 200' ||
  "$second_like" != *'"like_count":2,"liked_by_me":true}'*' 200' ]]; then
  echo "Likes did not count once per guest: $first_like | $repeated_like | $second_like" >&2
  exit 1
fi
top_response="$(curl -fsS -H "Cookie: $guest_cookie" "$smoke_base_url/api/media/top")"
if [[ "$top_response" != '{"items":[{"id":"'"$smoke_photo_id"'"'* || "$top_response" != *'"like_count":2,"liked_by_me":true}]}'* ]]; then
  echo "The top ranking does not list only the liked photo with two likes: $top_response" >&2
  exit 1
fi
if [[ "$(moderate hidden)" != 200 || "$(like PUT "$guest_cookie")" != *' 404' ||
  "$(curl -fsS -H "Cookie: $guest_cookie" "$smoke_base_url/api/media/top")" != '{"items":[]}' ||
  "$(moderate visible)" != 200 ]]; then
  echo 'A hidden photo could still be liked or stayed in the ranking.' >&2
  exit 1
fi
docker compose restart app >/dev/null
for _ in $(seq 1 60); do
  curl -fsS "$smoke_base_url/api/health" >/dev/null 2>&1 && break
  sleep 1
done
unliked="$(like DELETE "$second_guest_cookie")"
if [[ "$unliked" != *'"like_count":1,"liked_by_me":false}'*' 200' ]]; then
  echo "Likes did not survive a restart or unlike failed: $unliked" >&2
  exit 1
fi
echo 'Likes per guest, ranking, hidden exclusion and persistence across restart passed.'

# Pinned posts: admin creates with an image, guests see it, hiding the photo drops only the image, cap of 5, reorder and delete.
pinned_admin() {
  curl -sS -w ' %{http_code}' -X "$1" -H "Cookie: $admin_cookie" -H 'Content-Type: application/json' \
    ${3:+--data "$3"} "$smoke_base_url/api/admin/pinned$2"
}
guest_pinned() {
  curl -fsS -H "Cookie: $guest_cookie" "$smoke_base_url/api/pinned"
}
# The pinned image needs a published thumbnail, so give the smoke photo one first.
thumb_status="$(printf '\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00\xff\xd9' |
  curl -sS -o /dev/null -w '%{http_code}' -X POST -H "Cookie: $guest_cookie" -H 'Content-Type: image/jpeg' \
    --data-binary @- "$smoke_base_url/api/media/$smoke_photo_id/derived?variant=t")"
if [[ "$thumb_status" != 20[01] ]]; then
  echo "Uploading the smoke photo thumbnail returned HTTP $thumb_status; want 200 or 201." >&2
  exit 1
fi
created="$(pinned_admin POST '' '{"title":"Horario","body":"Ceremonia a las 12:00\nBanquete a las 14:00","media_id":"'"$smoke_photo_id"'"}')"
pinned_id="$(printf '%s' "$created" | sed -n 's/^{"id":"\([0-9a-f-]\{36\}\)".*/\1/p')"
if [[ "$created" != *' 201' || -z "$pinned_id" ]]; then
  echo "Creating a pinned post did not return 201 with an id: $created" >&2
  exit 1
fi
if [[ "$(curl -sS -o /dev/null -w '%{http_code}' "$smoke_base_url/api/pinned")" != 401 ||
  "$(curl -sS -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' --data '{"title":"x","body":"","media_id":null}' "$smoke_base_url/api/admin/pinned")" != 401 ]]; then
  echo 'Pinned routes answered without the guest or admin session.' >&2
  exit 1
fi
visible_pinned="$(guest_pinned)"
if [[ "$visible_pinned" != '{"items":[{"id":"'"$pinned_id"'","title":"Horario","body":"Ceremonia a las 12:00\nBanquete a las 14:00","image":{"id":"'"$smoke_photo_id"'"'* ]]; then
  echo "Guests do not see the pinned post with its image: $visible_pinned" >&2
  exit 1
fi
if [[ "$(moderate hidden)" != 200 || "$(guest_pinned)" != *'"image":null}]}' || "$(moderate visible)" != 200 ]]; then
  echo 'Hiding the referenced photo did not drop only the pinned image.' >&2
  exit 1
fi
for n in 2 3 4 5; do
  if [[ "$(pinned_admin POST '' '{"title":"Aviso '"$n"'","body":"","media_id":null}')" != *' 201' ]]; then
    echo "Creating pinned post $n failed." >&2
    exit 1
  fi
done
sixth="$(pinned_admin POST '' '{"title":"Sobra","body":"","media_id":null}')"
if [[ "$sixth" != '{"error":"pinned_limit_reached"}'*' 409' || "$(guest_pinned | grep -o '"id":"[0-9a-f-]\{36\}","title"' | wc -l)" != 5 ]]; then
  echo "A sixth pinned post was not rejected with 409 or changed the list: $sixth" >&2
  exit 1
fi
order_ids="$(guest_pinned | grep -o '"id":"[0-9a-f-]\{36\}","title"' | sed 's/"id":"\([^"]*\)".*/\1/' | tac | sed 's/.*/"&"/' | paste -sd, -)"
if [[ "$(pinned_admin PUT /order '{"ids":['"$order_ids"']}')" != *' 200' ||
  "$(guest_pinned)" != '{"items":[{"id":"'"$(printf '%s' "$order_ids" | cut -d, -f1 | tr -d '"')"'"'* ]]; then
  echo 'Reordering pinned posts did not change the guest order.' >&2
  exit 1
fi
if [[ "$(pinned_admin DELETE "/$pinned_id")" != ' 204' || "$(pinned_admin DELETE "/$pinned_id")" != *' 404' ||
  "$(guest_pinned)" == *"$pinned_id"* ]]; then
  echo 'Deleting a pinned post did not remove it exactly once.' >&2
  exit 1
fi
echo 'Pinned posts sessions, image visibility, cap of 5, reorder and delete passed.'
