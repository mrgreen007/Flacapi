# FLAC API HTTP Service — API Usage Documentation

This document serves as the comprehensive API reference for clients interacting with the FLAC API HTTP Service. All API endpoints use standard JSON payloads and return structured JSON responses.

---

## 1. General Configuration & Health

### Health Check
Verify the server is online and the API is reachable.

* **Endpoint**: `GET /health`
* **Response Status**: `200 OK`
* **Response Format (Normal / Online)**:
  ```json
  {
    "status": "ok",
    "upstream": {
      "status": "ok"
    }
  }
  ```

* **Response Format (Upstream Maintenance)**:
  ```json
  {
    "status": "ok",
    "upstream": {
      "status": "maintenance",
      "message": "Maintenance is in progress while we stabilize the server. Please try again later.",
      "maintenance_until": "2026-05-25T04:15:35.000Z"
    }
  }
  ```

* **Response Format (Upstream Offline / Unreachable)**:
  ```json
  {
    "status": "ok",
    "upstream": {
      "status": "offline",
      "error": "Get \"https://api.zarz.moe/v1/health\": context deadline exceeded"
    }
  }
  ```

---

## 2. Search & Downloads API

### Custom Search via Extension
Query paginated artist tracks directly from the selected SpotiFLAC extension's music service catalog.

* **Endpoint**: `POST /api/v1/search`
* **Request Headers**: `Content-Type: application/json`
* **Request Payload**:
  ```json
  {
    "service": "tidal-web",
    "query": "Arijit Singh",
    "options": {
      "type": "track"
    }
  }
  ```
* **Success Response Format**:
  ```json
  [
    {
      "id": "12345678",
      "name": "Tum Hi Ho",
      "artists": "Arijit Singh",
      "album_name": "Aashiqui 2",
      "album_artist": "Arijit Singh",
      "duration_ms": 262000,
      "images": "https://resources.tidal.com/images/1280x1280.jpg",
      "release_date": "2019-12-29",
      "track_number": 1,
      "total_tracks": 10,
      "disc_number": 1,
      "total_discs": 1,
      "isrc": "QM22L1901797",
      "provider_id": "tidal",
      "item_type": "track",
      "album_type": "album",
      "audio_quality": "HI_RES"
    }
  ]
  ```

---

### Download by Strategy
The primary endpoint to initiate a background search, resolution, and download of audio tracks using native fallback strategies or SpotiFLAC extensions. This endpoint executes asynchronously and returns immediately.

* **Endpoint**: `POST /api/v1/download/strategy`
* **Request Headers**: `Content-Type: application/json`
* **Request Payload**:
  ```json
  {
    "track_name": "Tum Hi Ho",
    "artist_name": "Arijit Singh",
    "album_name": "Aashiqui 2",
    "isrc": "QM22L1901797",
    "quality": "LOSSLESS",
    "use_extensions": true,
    "service": "tidal-web",
    "conversion_strategy": "FORCE_FLAC",
    "embed_metadata": true,
    "embed_lyrics": true,
    "embed_max_quality_cover": true
  }
  ```

#### **Payload Parameters Reference Table**

| Field | Type | Required | Description | Default / Allowed Values |
| :--- | :--- | :--- | :--- | :--- |
| `track_name` | string | Conditional* | The title of the track to search and download. | Optional ONLY if `isrc` supplied. |
| `artist_name` | string | Conditional* | The artist name of the track. | Optional ONLY if `isrc` supplied. |
| `album_name` | string | No | The album name to match against results. | Optional |
| `isrc` | string | No | Optional high-precision unique lookup key. If supplied, names become optional. | Optional |
| `quality` | string | No | Target audio download quality. If set to `LOSSLESS` or `HI_RES`, the API explicitly filters out lossy providers from the fallback chain. | Default: `LOSSLESS` (Allowed: `LOW`, `MEDIUM`, `HIGH`, `LOSSLESS`, `HI_RES`, `HI_RES_LOSSLESS`) |
| `use_extensions` | boolean | No | Enable the SpotiFLAC JS extension run-times. | Default: `true` (Allowed: `true`, `false`) |
| `service` | string | No | The specific extension provider to utilize first. | Default: fallback priority (Allowed: `amazon`, `apple-music`, `deezer`, `pandora`, `qobuz-web`, `soundcloud`, `spotify-web`, `tidal-web`, `ytmusic-spotiflac`) |
| `use_fallback` | boolean | No | Enable automatic fallback to other active extensions if the chosen provider fails. If `quality` is `LOSSLESS`, fallback is strictly restricted to lossless providers only. | Default: `true` (Allowed: `true`, `false`) |
| `conversion_strategy` | string | No | Choose whether to convert lossless `.m4a` to `.flac`. Lossy `.m4a` is never transcoded. | Default: `ORIGINAL` (Allowed: `ORIGINAL`, `FORCE_FLAC`) |
| `embed_metadata` | boolean | No | Embed ID3v2/Vorbis tags into the audio container. | Default: `true` (Allowed: `true`, `false`) |
| `embed_lyrics` | boolean | No | Fetch and embed synchronized LRC lyrics if available. | Default: `true` (Allowed: `true`, `false`) |
| `embed_max_quality_cover` | boolean | No | Download and embed high-resolution cover photo. | Default: `true` (Allowed: `true`, `false`) |
| `item_id` | string | No | Unique handle to distinctly track this specific download through asynchronous polling APIs. | Default: Generated automatically (e.g. `dl-1716480000000`) |

> [!NOTE]
> **Client-specified Output Directory**: The client-supplied `output_dir` parameter is discontinued and ignored. Staging and final library directory structures are managed internally by the server.

#### **Success Response Format**
Returns an asynchronous accepted confirmation containing the `itemId` immediately.

* **Response Status**: `202 Accepted`
```json
{
  "success": true,
  "itemId": "dl-1716480000000",
  "status": "preparing"
}
```

#### **Error Response Format (Not Found)**
```json
{
  "success": false,
  "error": "No extension download providers available",
  "error_type": "not_found"
}
```

---

## 3. Progress Tracking & Lifecycles

### Get Single Progress
Retrieve the progress status of a specific background download task by passing its `itemId` as a query parameter.

* **Endpoint**: `GET /api/v1/download/progress?itemId=<itemId>`
* **Response Format**:
  ```json
  {
    "item_id": "dl-1716480000000",
    "status": "downloading",
    "progress": 45.5,
    "speed_mbps": 2.4,
    "bytes_total": 10521883,
    "bytes_received": 4781232,
    "is_downloading": true,
    "cover_art_failed": false
  }
  ```

> [!TIP]
> **Status Lifecycle**: The status will progress from `preparing` $\rightarrow$ `downloading` $\rightarrow$ `finalizing` (transcoding & tagging) $\rightarrow$ `completed` or `failed`. If the download fails, the response will include an `error` field detailing the failure (e.g. `quality_rejected: Provided stream failed final lossless assertion test` if the provider returned lossy audio when lossless was requested).

### Get All Active Progresses
Retrieve all active and completed download progresses in the system.

* **Endpoint**: `GET /api/v1/download/progress/all`
* **Response Format**:
  ```json
  {
    "items": {
      "item-001": {
        "item_id": "item-001",
        "progress": 0.455,
        "speed_mbps": 2.4,
        "is_downloading": true,
        "status": "downloading",
        "bytes_total": 1000000
      }
    }
  }
  ```

### Get Progress Delta (Polling)
Optimized delta polling for UI clients, returning only progress changes since a particular sequence number.

* **Endpoint**: `GET /api/v1/download/progress/delta?since=<sequence_number>`
* **Response Format**:
  ```json
  {
    "last_seq": 102,
    "deltas": [
      {
        "id": "item-001",
        "percent": 48.2
      }
    ]
  }
  ```

### Initialize Item Progress
Initialize background progress tracking for a specific download item ID.

* **Endpoint**: `POST /api/v1/download/item/init`
* **Request Payload**:
  ```json
  {
    "itemId": "item-001"
  }
  ```
* **Response**:
  ```json
  {
    "success": true
  }
  ```

### Finish Item Progress
Mark the progress tracking of a download item ID as finished/completed.

* **Endpoint**: `POST /api/v1/download/item/finish`
* **Request Payload**:
  ```json
  {
    "itemId": "item-001"
  }
  ```
* **Response**:
  ```json
  {
    "success": true
  }
  ```

### Clear Item Progress
Remove progress tracking state/records of a completed item from the memory cache.

* **Endpoint**: `POST /api/v1/download/item/clear`
* **Request Payload**:
  ```json
  {
    "itemId": "item-001"
  }
  ```
* **Response**:
  ```json
  {
    "success": true
  }
  ```

### Cancel Download Item
Cancel an active background download task by its item ID.

* **Endpoint**: `POST /api/v1/download/item/cancel`
* **Request Payload**:
  ```json
  {
    "itemId": "item-001"
  }
  ```
* **Response**:
  ```json
  {
    "success": true
  }
  ```

---

## 4. File Delivery & Retrieval API

### Retrieve Finalized File
Download the completed audio file from the server. Once successfully downloaded by the client, the file is automatically purged from the server's disk to free up resources.

* **Endpoint**: `GET /api/v1/download/file?itemId=<itemId>`
* **Response Status**: 
  * `200 OK` (Streams file binary)
  * `400 Bad Request` (If download is still in progress)
  * `410 Gone` (If download failed or has already been retrieved/purged)
* **Response Headers**:
  * `Content-Type: application/octet-stream`
  * `Content-Disposition: attachment; filename="Arijit Singh - Tum Hi Ho.flac"`

---

## 5. Lyrics Management

### Get LRC Lyrics
Fetch synchronized LRC lyrics for a track from Spotify ID, names, and duration.

* **Endpoint**: `POST /api/v1/lyrics/get`
* **Request Payload**:
  ```json
  {
    "spotifyId": "4pt7mS6v697u697",
    "trackName": "Tum Hi Ho",
    "artistName": "Arijit Singh",
    "durationMs": 262000
  }
  ```
* **Response Format**:
  ```json
  {
    "lyrics": "[00:10.50]Hum tere bin ab reh nahi sakte...",
    "source": "Musixmatch",
    "sync_type": "LINE_SYNCED",
    "instrumental": false
  }
  ```

---

## 6. Catalog & Availability API

### Check Track Availability
Check if a track is available across platforms using its Spotify ID or ISRC code.

* **Endpoint**: `POST /api/v1/catalog/availability`
* **Payload**:
  ```json
  {
    "spotifyId": "4pt7mS6v697u697",
    "isrc": "QM22L1901797"
  }
  ```
* **Response**:
  ```json
  {
    "spotify_id": "4pt7mS6v697u697",
    "tidal": true,
    "amazon": true,
    "qobuz": false,
    "deezer": true,
    "youtube": true,
    "tidal_id": "12345678",
    "deezer_id": "87654321",
    "tidal_url": "https://tidal.com/track/...",
    "deezer_url": "https://deezer.com/track/..."
  }
  ```

### Check Availability By Platform ID
Check if an entity (track, album, playlist) is available on a specific music platform.

* **Endpoint**: `POST /api/v1/catalog/availability/platform`
* **Payload**:
  ```json
  {
    "platform": "tidal",
    "entityType": "track",
    "entityId": "12345678"
  }
  ```
* **Response**:
  ```json
  {
    "available": true
  }
  ```

### Cross-Platform ID Resolution
Resolve a Spotify ID into a corresponding Deezer ID.

* **Endpoint**: `POST /api/v1/catalog/resolve-id`
* **Payload**:
  ```json
  {
    "resourceType": "track",
    "spotifyId": "4pt7mS6v697u"
  }
  ```
* **Response**:
  ```json
  {
    "deezer_id": "123456"
  }
  ```

### Fetch Provider Metadata
Fetch raw metadata from any supported extension provider.

* **Endpoint**: `POST /api/v1/catalog/metadata`
* **Payload**:
  ```json
  {
    "providerId": "tidal-web",
    "resourceType": "track",
    "resourceId": "12345678"
  }
  ```
* **Response**:
  ```json
  {
    "track": {
      "id": "12345678",
      "name": "Tum Hi Ho",
      "artists": "Arijit Singh",
      "duration_ms": 262000,
      "provider_id": "tidal-web"
    }
  }
  ```

---

## 7. Client Integration Examples

### PowerShell
```powershell
$body = @{
    track_name = "Tum Hi Ho"
    artist_name = "Arijit Singh"
    album_name = "Aashiqui 2"
    quality = "LOSSLESS"
    use_extensions = $true
    service = "tidal-web"
    embed_metadata = $true
    embed_max_quality_cover = $true
} | ConvertTo-Json

$response = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/download/strategy" -Method Post -Body $body -ContentType "application/json"
$response | Format-List
```

### Curl (Bash / CLI)
```bash
curl -X POST http://localhost:8080/api/v1/download/strategy \
     -H "Content-Type: application/json" \
     -d '{
       "track_name": "Tum Hi Ho",
       "artist_name": "Arijit Singh",
       "album_name": "Aashiqui 2",
       "quality": "LOSSLESS",
       "use_extensions": true,
       "service": "tidal-web",
       "embed_metadata": true,
       "embed_max_quality_cover": true
     }'
```

---

## 🏁 8. Example API Call Flow

Here is a comprehensive blueprint of a standard end-to-end interactive UI lifecycle using the Flacapi interface:

### Step 1: Ensure Server Readiness
The client initializes by verifying connectivity to the active server and checking system status:
* **Request**: `GET http://localhost:8080/health` (or similar baseline GET ping)
* **Pre-Flight Check**: Verify status is `"ok"` and inspect upstream dependency availability.

### Step 2: Discover Song via Direct Search
A user enters **"Arijit Singh"** into your interface search bar. The client triggers a provider lookup to gather accurate results:
* **Request**: `POST http://localhost:8080/api/v1/search`
* **Payload**:
  ```json
  {
    "service": "apple-music",
    "query": "Arijit",
    "options": {
      "type": "track"
    }
  }
  ```
* **Discovery**: The search returns detailed track object lists. The client extracts the target's unique persistent identifier (**ISRC: `"QM22L1901797"`**) from the list results.

### Step 3: Check Catalog Availability (Using ISRC Only)
Before starting, the client queries the aggregator engine using ONLY the ISRC to confirm licensing availability across lossless networks:
* **Request**: `POST http://localhost:8080/api/v1/catalog/availability`
* **Payload**:
  ```json
  {
    "isrc": "QM22L1901797"
  }
  ```
* **Analysis**: The engine confirms the ISRC is fully active and playable on `tidal: true` and `amazon: true`.

### Step 4: Dispatch Master Quality Download (Smart Async Route)
The client initiates the download. Crucially, the endpoint returns immediately with a tracking `itemId`:
* **Request**: `POST http://localhost:8080/api/v1/download/strategy`
* **Payload**:
  ```json
  {
    "isrc": "QM22L1901797",
    "quality": "LOSSLESS",
    "use_extensions": true,
    "conversion_strategy": "FORCE_FLAC"
  }
  ```
* **Response**: `202 Accepted` returning `{"success": true, "itemId": "dl-12345", "status": "preparing"}`.

### Step 5: Poll Progress Until Completed
A background thread polls the filtered progress endpoint using the returned `itemId`:
* **Request**: `GET http://localhost:8080/api/v1/download/progress?itemId=dl-12345`
* **Completed Poll Return**:
  ```json
  {
    "item_id": "dl-12345",
    "status": "completed",
    "progress": 100.0,
    "speed_mbps": 0.0,
    "is_downloading": false,
    "cover_art_failed": false,
    "bytes_received": 10521883,
    "bytes_total": 10521883
  }
  ```

### Step 6: Pull Finished File over HTTP
Once status is `completed`, the client downloads the track directly from the server. The server deletes the file from disk upon successful transfer completion:
* **Request**: `GET http://localhost:8080/api/v1/download/file?itemId=dl-12345`
* **Response**: `200 OK` with binary file stream.
* **Victory State**: The client has the fully tagged, verified lossless FLAC file locally, and the server's disk space is instantly reclaimed!

---

## 9. System Administration & Environment Configuration

The FLAC API Server is highly configurable via standard System Environment Variables. You can define these in a local `.env` file in the server root or export them directly in your Docker container stack.

| Environment Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `FLACAPI_CONVERSION_STRATEGY` | string | `ORIGINAL` | Set to `ORIGINAL` to deliver raw native lossless formats (e.g. `.m4a`). Set to `FORCE_FLAC` to force internal FFmpeg conversion to `.flac` containers always. |
| `FLACAPI_PROVIDER_PRIORITY` | list | *See Default* | Comma-separated string explicitly prioritizing extension traversal (e.g., `qobuz-web,apple-music`). |
| `FLACAPI_AUTO_UPDATE_EXTENSIONS` | boolean | `true` | Enable or disable dynamic mirror synchronization on startup. Set to `false` to lock package versions. |
| `FLACAPI_EXTENSION_STORE_URL` | string | (Public Repo) | Override the SpotiFLAC community repository URL with your own manual link or local file tree. |
| `FLACAPI_APPLE_PROXY_KEY` | string | (Empty) | Custom authorized key for third-party premium Apple Music proxy nodes. |
| `FLACAPI_TIDAL_MIRROR_URL` | string | (Empty) | Hard-override standard Tidal web scraper mirror with your own custom private endpoint URL. |
| `FLACAPI_TIDAL_TOKEN` | string | (Empty) | Custom Tidal Public Client Token injected directly into backend scraping request streams. |
| `FLACAPI_RETENTION_HOURS` | integer | `2` | Number of hours completed or failed download tasks/files are retained on the server before automated cleanup sweeps. |

#### **Standard Provider Baseline Sequence**
If `FLACAPI_PROVIDER_PRIORITY` is not explicitly declared, the engine traverses loaded services in the following persistent hierarchy:
1. `apple-music`
2. `tidal-web`
3. `qobuz-web`
4. `deezer`
5. `amazon`
6. `ytmusic-spotiflac`
7. `soundcloud`
8. `pandora`
