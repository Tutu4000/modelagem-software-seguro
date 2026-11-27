#!/usr/bin/env bash
# Joins the clips of `just demo` into fluxo-capitulos.mp4: a caption with the
# chapter's number and name over the video, and the chapters in the MP4
# itself (players show them on the timeline). Each clip has a .chapters file
# with "seconds<TAB>title" lines, written by the spec.
set -euo pipefail
cd "$(dirname "$0")/out"

trim=0.8 # the start of each clip is the page loading
font="$(fc-match -f '%{file}' 'sans:bold')"
rm -f cap-*.mp4 cap-*.txt capitulos.txt list.txt meta.txt fluxo-capitulos.mp4

n=0
offset=0
: >meta.txt
printf ';FFMETADATA1\n' >>meta.txt
for chapters in 0*.chapters; do
    clip="${chapters%.chapters}"
    duration="$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$clip.webm")"
    length="$(awk -v d="$duration" -v t="$trim" 'BEGIN { print d - t }')"
    mapfile -t lines <"$chapters"
    filter="scale=1920:1080,fps=25"
    for i in "${!lines[@]}"; do
        n=$((n + 1))
        start="$(cut -f1 <<<"${lines[$i]}")"
        title="$(cut -f2 <<<"${lines[$i]}")"
        if [ "$i" -eq 0 ]; then start=0; else start="$(awk -v s="$start" -v t="$trim" 'BEGIN { s -= t; print (s < 0 ? 0 : s) }')"; fi
        if [ "$i" -lt $((${#lines[@]} - 1)) ]; then
            end="$(cut -f1 <<<"${lines[$((i + 1))]}" | awk -v t="$trim" '{ print $1 - t }')"
        else
            end="$length"
        fi
        printf '%s. %s' "$n" "$title" >"cap-$n.txt"
        filter="$filter,drawtext=fontfile=$font:textfile=cap-$n.txt:fontsize=44:fontcolor=black:box=1:boxcolor=white@0.92:boxborderw=20:x=36:y=h-th-40:enable='between(t,$start,$end)'"
        # The chapter's place in the final video, in milliseconds.
        from="$(awk -v o="$offset" -v s="$start" 'BEGIN { printf "%d", (o + s) * 1000 }')"
        to="$(awk -v o="$offset" -v e="$end" 'BEGIN { printf "%d", (o + e) * 1000 }')"
        printf '[CHAPTER]\nTIMEBASE=1/1000\nSTART=%s\nEND=%s\ntitle=%s. %s\n' "$from" "$to" "$n" "$title" >>meta.txt
        printf '%s\t%s. %s\n' "$(awk -v o="$offset" -v s="$start" 'BEGIN { printf "%d:%02d", (o + s) / 60, (o + s) % 60 }')" "$n" "$title" >>capitulos.txt
    done
    ffmpeg -v error -y -ss "$trim" -i "$clip.webm" -vf "$filter" -c:v libx264 -crf 24 -pix_fmt yuv420p -an "cap-$clip.mp4"
    offset="$(awk -v o="$offset" -v l="$length" 'BEGIN { print o + l }')"
done

printf "file '%s'\n" cap-0*.mp4 >list.txt
ffmpeg -v error -y -f concat -safe 0 -i list.txt -i meta.txt -map_metadata 1 -map_chapters 1 -c copy fluxo-capitulos.mp4
rm -f cap-*.mp4 cap-*.txt list.txt meta.txt
cat capitulos.txt
ls -lh fluxo-capitulos.mp4
