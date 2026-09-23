#!/bin/sh
# downloads the pinned third-party frontend assets into a frontend dir
# usage: fetch-assets.sh <frontend dir>
set -e

dest=${1:?usage: fetch-assets.sh <frontend dir>}
mkdir -p "$dest/css" "$dest/scripts"

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$dest/$1" "$2"
	else
		wget -q -O "$dest/$1" "$2"
	fi
}

fetch css/flatpickr.min.css "https://cdn.jsdelivr.net/npm/flatpickr@4.6.13/dist/flatpickr.min.css"
fetch scripts/axios.min.js "https://cdn.jsdelivr.net/npm/axios@1.2.2/dist/axios.min.js"
fetch scripts/flatpickr.min.js "https://cdn.jsdelivr.net/npm/flatpickr@4.6.13/dist/flatpickr.min.js"
fetch scripts/chart.min.js "https://cdnjs.cloudflare.com/ajax/libs/Chart.js/3.5.1/chart.min.js"
fetch scripts/hammer.min.js "https://cdnjs.cloudflare.com/ajax/libs/hammer.js/2.0.8/hammer.min.js"
fetch scripts/chartjs-plugin-zoom.min.js "https://cdnjs.cloudflare.com/ajax/libs/chartjs-plugin-zoom/1.1.1/chartjs-plugin-zoom.min.js"
fetch scripts/moment.min.js "https://cdn.jsdelivr.net/npm/moment@2.27.0"
fetch scripts/moment-timezone-with-data.min.js "https://cdn.jsdelivr.net/npm/moment-timezone@0.5.17/builds/moment-timezone-with-data.min.js"
fetch scripts/chartjs-adapter-moment.min.js "https://cdn.jsdelivr.net/npm/chartjs-adapter-moment@0.1.1"
