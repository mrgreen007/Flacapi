# FLAC API HTTP Service — API Usage Documentation

This document serves as the comprehensive API reference for clients interacting with the FLAC API HTTP Service. All API endpoints use standard JSON payloads and return structured JSON responses.

---

## 1. General Configuration & Health

### Health Check
Verify the server is online and the API is reachable.

* **Endpoint**: `GET /health`
* **Response Status**: `200 OK`
* **Response Format**:
  ```json
  {
    "status": "ok"
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
The primary endpoint to search, resolve, and download audio tracks using native fallback strategies or SpotiFLAC extensions.

* **Endpoint**: `POST /api/v1/download/strategy`
* **Request Headers**: `Content-Type: application/json`
* **Request Payload**:
  ```json
  {
    "track_name": "Tum Hi Ho",
    "artist_name": "Arijit Singh",
    "album_name": "Aashiqui 2",
    "isrc": "QM22L1901797",
    "output_dir": "./data/output",
    "output_ext": ".flac",
    "quality": "LOSSLESS",
    "use_extensions": true,
    "service": "tidal-web",
    "embed_metadata": true,
    "embed_lyrics": true,
    "embed_max_quality_cover": true
  }
  ```

#### **Payload Parameters Reference Table**

| Field | Type | Required | Description | Default / Allowed Values |
| :--- | :--- | :--- | :--- | :--- |
| `track_name` | string | Yes | The title of the track to search and download. | |
| `artist_name` | string | Yes | The artist name of the track. | |
| `album_name` | string | No | The album name to match against results. | |
| `isrc` | string | No | Optional high-precision lookup key. Recommended for direct unique matches. | |
| `output_dir` | string | No | Relative or absolute path where the final audio file will be saved. | Relative paths automatically resolve to server root. |
| `output_ext` | string | No | Target extension for conversion. | `.flac`, `.m4a`, `.opus` |
| `quality` | string | No | Target audio download quality. If set to `LOSSLESS`, the API explicitly filters out lossy providers from the fallback chain. | `LOW`, `MEDIUM`, `HIGH`, `LOSSLESS` |
| `use_extensions` | boolean | No | Enable the SpotiFLAC JS extension run-times. | `true`, `false` |
| `service` | string | No | The specific extension provider to utilize. | `amazon`, `apple-music`, `deezer`, `pandora`, `qobuz-web`, `soundcloud`, `spotify-web`, `tidal-web`, `ytmusic-spotiflac` |
| `use_fallback` | boolean | No | Enable automatic fallback to other active extensions if the chosen provider fails. If `quality` is `LOSSLESS`, fallback is strictly restricted to lossless providers only. | `true`, `false` (default: `false`) |
| `embed_metadata` | boolean | No | Embed ID3v2/Vorbis tags into the audio container. | `true`, `false` |
| `embed_lyrics` | boolean | No | Fetch and embed synchronized LRC lyrics if available. | `true`, `false` |
| `embed_max_quality_cover` | boolean | No | Download and embed high-resolution cover photo. | `true`, `false` |

> [!TIP]
> **Smart API Enhancements Powered by the Flacapi Engine:**
> * **Auto-Resolve ISRC**: If you supply an `isrc` but omit track/artist names, the server instantly invokes the core metadata library to transparently resolve and populate the correct track details before beginning the download.
> * **Safe Directory Routing**: If you omit `output_dir`, the server intelligently roots the operation into your globally configured server downloads directory instead of failing or creating stray local temp files.
> * **Filename Sanitization**: The API layer proactively intercepts and scrubs illegal Windows filesystem tokens (like `< > : " / \ | ? *`) from your inputs, preventing fatal disk creation errors.

#### **Success Response Format (Lossless ALAC/Tidal)**
```json
{
  "success": true,
  "message": "Downloaded from tidal-web",
  "file_path": "C:\\Users\\sabuj\\Workspace\\Projects\\Sabuj.in\\Flacapi\\data\\output\\Arijit Singh - Tum Hi Ho.m4a",
  "actual_bit_depth": 16,
  "actual_sample_rate": 44100,
  "service": "tidal-web",
  "title": "Tum Hi Ho (From \"Aashiqui 2\")",
  "artist": "Arijit Singh",
  "album": "Aashiqui 2",
  "album_artist": "Arijit Singh",
  "release_date": "2019-12-29",
  "track_number": 1,
  "disc_number": 1,
  "isrc": "QM22L1901797",
  "cover_url": "https://resources.tidal.com/images/16bd0ffc/1681/4568/919c/9dc4ba55176f/1280x1280.jpg",
  "copyright": "2019 Arijit Singh"
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
Retrieve the progress status of the currently active download.

* **Endpoint**: `GET /api/v1/download/progress`
* **Response Format**:
  ```json
  {
    "current_file": "12345",
    "progress": 45.5,
    "speed_mbps": 2.4,
    "bytes_total": 10521883,
    "bytes_received": 4781232,
    "is_downloading": true,
    "status": "downloading"
  }
  ```

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

## 4. Metadata & Lyrics Management

### Read File Metadata
Read tags and embedded cover art metadata from a downloaded file on disk.

* **Endpoint**: `POST /api/v1/metadata/read`
* **Request Payload**:
  ```json
  {
    "filePath": "./data/output/Arijit Singh - Tum Hi Ho.m4a"
  }
  ```
* **Response Format**:
  ```json
  {
    "title": "Tum Hi Ho",
    "artist": "Arijit Singh",
    "album": "Aashiqui 2",
    "album_artist": "Arijit Singh",
    "date": "2019-12-29",
    "track_number": 1,
    "total_tracks": 10,
    "disc_number": 1,
    "isrc": "QM22L1901797",
    "genre": "Bollywood",
    "duration": 262
  }
  ```

### Edit File Metadata
Update tags on an existing audio file on disk.

* **Endpoint**: `POST /api/v1/metadata/edit`
* **Request Payload**:
  ```json
  {
    "file_path": "./data/output/Arijit Singh - Tum Hi Ho.m4a",
    "title": "Tum Hi Ho (Special Edition)",
    "artist": "Arijit Singh",
    "album": "Aashiqui 2"
  }
  ```
* **Response Format**:
  ```json
  {
    "success": true,
    "message": "Metadata updated successfully"
  }
  ```

### Download Cover Art
Download track cover art from a URL into a safe local file path.

* **Endpoint**: `POST /api/v1/metadata/cover`
* **Request Payload**:
  ```json
  {
    "coverUrl": "https://resources.tidal.com/images/1280x1280.jpg",
    "outputPath": "./data/output/cover.jpg",
    "maxQuality": true
  }
  ```
* **Response Format**:
  ```json
  {
    "success": true
  }
  ```

### Get LRC Lyrics
Fetch synchronized LRC lyrics for a track from Spotify ID, names, or a local audio file path containing embedded lyrics.

* **Endpoint**: `POST /api/v1/lyrics/get`
* **Request Payload**:
  ```json
  {
    "spotifyId": "4pt7mS6v697u697",
    "trackName": "Tum Hi Ho",
    "artistName": "Arijit Singh",
    "filePath": "./data/output/Arijit Singh - Tum Hi Ho.m4a",
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

### Embed Lyrics to File
Embed raw or synchronized LRC lyrics directly into the metadata of an audio file on disk.

* **Endpoint**: `POST /api/v1/lyrics/embed`
* **Request Payload**:
  ```json
  {
    "filePath": "./data/output/Arijit Singh - Tum Hi Ho.m4a",
    "lyrics": "[00:10.50]Hum tere bin ab reh nahi sakte..."
  }
  ```
* **Response Format**:
  ```json
  {
    "success": true,
    "message": "Lyrics embedded successfully"
  }
  ```

---

## 5. Catalog & Availability API

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

## 6. Deduplication API

### Check Single Duplicate
Check if a track has already been downloaded using its ISRC.

* **Endpoint**: `POST /api/v1/download/duplicate/check`
* **Payload**:
  ```json
  {
    "outputDir": "./data/output",
    "isrc": "QM22L1901797"
  }
  ```
* **Response**:
  ```json
  {
    "exists": true,
    "filepath": "C:\\Users\\sabuj\\Workspace\\Projects\\Sabuj.in\\Flacapi\\data\\output\\Arijit Singh - Tum Hi Ho.m4a"
  }
  ```

### Check Duplicate Batch
Perform duplicate verification for a batch of tracks.

* **Endpoint**: `POST /api/v1/download/duplicate/check-batch`
* **Payload**:
  ```json
  {
    "outputDir": "./data/output",
    "tracksJSON": "[{\"isrc\":\"QM22L1901797\"}]"
  }
  ```
* **Response**:
  ```json
  [
    {
      "isrc": "QM22L1901797",
      "exists": true,
      "file_path": "./data/output/Arijit Singh - Tum Hi Ho.m4a"
    }
  ]
  ```

---

## 7. Advanced Cover Art & Cue Sheets

### Extract Embedded Cover Art
Extract embedded cover photo from a local audio file on disk.

* **Endpoint**: `POST /api/v1/metadata/extract-cover`
* **Payload**:
  ```json
  {
    "audioPath": "./data/output/Arijit Singh - Tum Hi Ho.m4a",
    "outputPath": "./data/output/cover.jpg"
  }
  ```
* **Response**:
  ```json
  {
    "success": true
  }
  ```

### Parse Lossless CUE Sheets
Parse a lossless `.cue` sheet file, returning song segments and subdivisions.

* **Endpoint**: `POST /api/v1/library/parse-cue`
* **Payload**:
  ```json
  {
    "cuePath": "./data/music/album.cue",
    "audioDir": "./data/music"
  }
  ```
* **Response**:
  ```json
  {
    "album": "Masterpieces",
    "artist": "Various Artists",
    "tracks": [
      {
        "index": 1,
        "title": "Intro",
        "start_time": "00:00:00"
      }
    ]
  }
  ```

## 8. Configuration API

### Set Default Download Directory
Configure the default target download directory safely within the allowed boundary.

* **Endpoint**: `POST /api/v1/config/download-dir`
* **Request Payload**:
  ```json
  {
    "path": "./data/downloads"
  }
  ```
* **Response Format**:
  ```json
  {
    "success": true
  }
  ```

---

## 9. Client Integration Examples

### PowerShell
```powershell
$body = @{
    track_name = "Tum Hi Ho"
    artist_name = "Arijit Singh"
    album_name = "Aashiqui 2"
    output_dir = "./data/output"
    output_ext = ".flac"
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
       "output_dir": "./data/output",
       "output_ext": ".flac",
       "quality": "LOSSLESS",
       "use_extensions": true,
       "service": "tidal-web",
       "embed_metadata": true,
       "embed_max_quality_cover": true
     }'
```

---

## 🏁 10. Example API Call Flow

This walkthrough demonstrates the step-by-step sequence of API calls to search, download, track, and locate a premium lossless song using the FLAC API Service.

### Step 1: Artist Track Search & Pagination (Client UI)
First, the client queries our newly exposed extension search API to search for an artist's tracks directly from Tidal/Deezer:

* **Request**: `POST http://localhost:8080/api/v1/search`
* **Payload**:
  ```json
  {
    "service": "tidal-web",
    "query": "Arijit Singh",
    "options": {
      "type": "track"
    }
  }
  ```
* **Response**:
  ```json
  [
    {
      "id": "12345678",
      "name": "Tum Hi Ho",
      "artists": ["Arijit Singh"],
      "album_name": "Aashiqui 2",
      "duration_ms": 262000
    },
    {
      "id": "87654321",
      "name": "Channa Mereya",
      "artists": ["Arijit Singh"],
      "album_name": "Ae Dil Hai Mushkil",
      "duration_ms": 289000
    }
  ]
  ```

### Step 2: User Picks a Track
The user selects a track from the paginated list (for example, **"Tum Hi Ho"**). The client extracts the precise metadata fields (`track_name`, `artist_name`, `album_name`) to prepare the download payload.

### Step 3: Dispatch Lossless Download Request
The client sends the precise track metadata to the FLAC API, requesting the maximum quality setting (**`LOSSLESS`**) via the Tidal extension to download the studio-master audio:

* **Request**: `POST http://localhost:8080/api/v1/download/strategy`
* **Request Headers**: `Content-Type: application/json`
* **Payload**:
  ```json
  {
    "track_name": "Tum Hi Ho",
    "artist_name": "Arijit Singh",
    "album_name": "Aashiqui 2",
    "output_dir": "./data/output",
    "output_ext": ".flac",
    "quality": "LOSSLESS",
    "use_extensions": true,
    "service": "tidal-web",
    "embed_metadata": true,
    "embed_max_quality_cover": true
  }
  ```

### Step 4: Track Real-Time Progress (Background Thread Polling)
While the download strategy endpoint is actively running in Step 3, the client UI starts a background polling thread to fetch real-time download status, percentages, speed, and ETA to display a progress bar to the user:

* **Request**: `GET http://localhost:8080/api/v1/download/progress`
* **Sample Response (During Download)**:
  ```json
  {
    "current_file": "item-abc",
    "progress": 55.4,
    "speed_mbps": 3.1,
    "is_downloading": true,
    "status": "downloading",
    "bytes_received": 5829103,
    "bytes_total": 10521883
  }
  ```

### Step 5: Final Success Response & File Retrieval
Once the download is completed, the download strategy request (Step 3) completes successfully and returns the final metadata along with the absolute local file path:

* **Response**:
  ```json
  {
    "success": true,
    "message": "Downloaded from tidal-web",
    "file_path": "C:\\Users\\sabuj\\Workspace\\Projects\\Sabuj.in\\Flacapi\\data\\output\\Arijit Singh - Tum Hi Ho.m4a",
    "actual_bit_depth": 16,
    "actual_sample_rate": 44100,
    "service": "tidal-web",
    "title": "Tum Hi Ho (From \"Aashiqui 2\")",
    "artist": "Arijit Singh",
    "album": "Aashiqui 2",
    "cover_url": "https://resources.tidal.com/images/16bd0ffc/1681/4568/919c/9dc4ba55176f/1280x1280.jpg"
  }
  ```
* **Locating the File**: The client can now play the 100% CD-Quality Lossless ALAC file (`Arijit Singh - Tum Hi Ho.m4a`) from `./data/output/` on disk!

