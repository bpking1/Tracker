package domain

type Status string

const (
	Planned  Status = "planned"
	Watching Status = "watching"
	Watched  Status = "watched"
	Dropped  Status = "dropped"
)

type MediaRef struct {
	Type  string `json:"type"`
	ID    int    `json:"id"`
	Title string `json:"title,omitempty"`
}

type ParseWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type MediaMetadata struct {
	MediaRef      MediaRef `json:"mediaRef"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"originalTitle"`
	ReleaseDate   string   `json:"releaseDate"`
	Overview      string   `json:"overview"`
	PosterURL     string   `json:"posterUrl"`
	Genres        []string `json:"genres"`
	Cast          []string `json:"cast"`
	VoteAverage   float64  `json:"voteAverage"`
	FetchedAt     string   `json:"fetchedAt"`
}

type Record struct {
	Key           string         `json:"key"`
	Status        Status         `json:"status"`
	Title         string         `json:"title"`
	MediaRef      *MediaRef      `json:"mediaRef"`
	CompletedAt   *string        `json:"completedAt"`
	CreatedAt     *string        `json:"createdAt"`
	Rating        *int           `json:"rating"`
	Progress      *string        `json:"progress"`
	Tags          []string       `json:"tags"`
	Comment       *string        `json:"comment"`
	RawLine       string         `json:"rawLine"`
	LineNumber    int            `json:"lineNumber"`
	Warnings      []ParseWarning `json:"warnings"`
	Metadata      *MediaMetadata `json:"metadata"`
	MetadataState string         `json:"metadataState"`
}

type RecordInput struct {
	Status      Status    `json:"status"`
	Title       string    `json:"title"`
	MediaRef    *MediaRef `json:"mediaRef"`
	CompletedAt *string   `json:"completedAt"`
	CreatedAt   *string   `json:"createdAt"`
	Rating      *int      `json:"rating"`
	Progress    *string   `json:"progress"`
	Tags        []string  `json:"tags"`
	Comment     *string   `json:"comment"`
}

type Snapshot struct {
	Revision     string         `json:"revision"`
	Records      []Record       `json:"records"`
	FileWarnings []ParseWarning `json:"fileWarnings"`
}
