module sabuj.in/flacapi

go 1.25.0

require (
	github.com/go-chi/chi/v5 v5.0.12
	github.com/gorilla/handlers v1.5.1
	github.com/zarz/spotiflac_android/go_backend v0.0.0-00010101000000-000000000000
)

require (
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/dop251/goja v0.0.0-20260311135729-065cd970411c // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/go-flac/flacpicture/v2 v2.0.2 // indirect
	github.com/go-flac/flacvorbis/v2 v2.0.2 // indirect
	github.com/go-flac/go-flac/v2 v2.0.4 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20260302011040-a15ffb7f9dcc // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/refraction-networking/utls v1.8.2 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/mobile v0.0.0-20260410095206-2cfb76559b7b // indirect
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	golang.org/x/tools v0.44.0 // indirect
)

// Replace the go_backend module with our vendored copy
replace github.com/zarz/spotiflac_android/go_backend => ./internal/go_backend
