package api

type StatusRequest struct {
	Data StatusData `json:"data"`
}

type StatusData struct {
	ID         string           `json:"id"`
	Type       string           `json:"type"`
	Attributes StatusAttributes `json:"attributes"`
}

type StatusAttributes struct {
	Paused  *bool `json:"paused,omitempty"`
	Stopped *bool `json:"stopped,omitempty"`
	Vus     *int  `json:"vus,omitempty"`
	VusMax  *int  `json:"vus-max,omitempty"`
}
