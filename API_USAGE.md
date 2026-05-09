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

## 2. Downloads & Strategy API

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
| `output_dir` | string | No | Relative or absolute path where the final audio file will be saved. | Relative paths automatically resolve to server root. |
| `output_ext` | string | No | Target extension for conversion. | `.flac`, `.m4a`, `.opus` |
| `quality` | string | No | Target audio download quality. | `LOW`, `MEDIUM`, `HIGH`, `LOSSLESS` |
| `use_extensions` | boolean | No | Enable the SpotiFLAC JS extension run-times. | `true`, `false` |
| `service` | string | No | The specific extension provider to utilize. | `amazon`, `apple-music`, `deezer`, `pandora`, `qobuz-web`, `soundcloud`, `spotify-web`, `tidal-web`, `ytmusic-spotiflac` |
| `embed_metadata` | boolean | No | Embed ID3v2/Vorbis tags into the audio container. | `true`, `false` |
| `embed_lyrics` | boolean | No | Fetch and embed synchronized LRC lyrics if available. | `true`, `false` |
| `embed_max_quality_cover` | boolean | No | Download and embed high-resolution cover photo. | `true`, `false` |

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

## 3. Progress tracking & Lifecycles

### Get Single Progress
Retrieve the progress status of the currently active download.

* **Endpoint**: `GET /api/v1/download/progress`
* **Response Format**:
  ```json
  {
    "active": true,
    "progress": 45.5,
    "speed": "2.4 MB/s",
    "eta": "00:08",
    "bytes_downloaded": 4781232,
    "total_bytes": 10521883
  }
  ```

### Get All Active Progresses
Retrieve all active and completed download progresses in the system.

* **Endpoint**: `GET /api/v1/download/progress/all`
* **Response Format**:
  ```json
  [
    {
      "id": "item-001",
      "title": "Tum Hi Ho",
      "status": "downloading",
      "percent": 45.5
    }
  ]
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

---

## 4. Metadata & Lyrics Management

### Read File Metadata
Read tags and embedded cover art metadata from a downloaded file on disk.

* **Endpoint**: `POST /api/v1/metadata/read`
* **Request Payload**:
  ```json
  {
    "file_path": "./data/output/Arijit Singh - Tum Hi Ho.m4a"
  }
  ```
* **Response Format**:
  ```json
  {
    "title": "Tum Hi Ho (From \"Aashiqui 2\")",
    "artist": "Arijit Singh",
    "album": "Aashiqui 2",
    "year": "2019",
    "track_number": "1",
    "has_cover": true
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

---

## 5. Client Integration Examples

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
