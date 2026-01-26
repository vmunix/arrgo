package v1

//go:generate mockgen -destination=mocks/mocks.go -package=mocks github.com/vmunix/arrgo/internal/api/v1 Searcher,DownloadManager,PlexClient,FileImporter,TVDBService
