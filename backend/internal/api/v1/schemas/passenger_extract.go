package schemas

// PassengerExtractRequest is everything the employee pasted into the browser extension at once:
// any text, and the images in the order they came. The model sorts out who is who. Images are
// base64, already shrunk by the extension; nothing in here is ever stored.
type PassengerExtractRequest struct {
	Text   string   `json:"text" validate:"max=20000"`
	Images []string `json:"images" validate:"max=20"`
}
