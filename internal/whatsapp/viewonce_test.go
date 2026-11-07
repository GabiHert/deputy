package whatsapp

import (
	"reflect"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestViewOnceKind(t *testing.T) {
	key, path := []byte{1, 2, 3}, proto.String("/v/t62/x")
	cases := []struct {
		name         string
		m            *waE2E.Message
		kind         string
		downloadable bool
	}{
		{"photo with media", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{MediaKey: key, DirectPath: path}}, "image", true},
		{"photo kept from companions", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, "image", false},
		{"photo without a path", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{MediaKey: key}}, "image", false},
		{"video", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{MediaKey: key, DirectPath: path}}, "video", true},
		{"voice note", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{PTT: proto.Bool(true)}}, "voice", false},
		{"audio", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{MediaKey: key, DirectPath: path}}, "audio", true},
		{"nothing inside", &waE2E.Message{}, "image", false},
		{"nil", nil, "image", false},
	}
	for _, c := range cases {
		kind, ok := viewOnceKind(c.m)
		if kind != c.kind || ok != c.downloadable {
			t.Errorf("%s: got %q %v", c.name, kind, ok)
		}
	}
}

func TestViewOncePlaceholder(t *testing.T) {
	var in Incoming
	m := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{Caption: proto.String("look"), Seconds: proto.Uint32(9),
		ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("Q1")}}}
	ci := viewOncePlaceholder(&in, m, "video")
	if in.Kind != "video" || !in.ViewOnce || in.Text != "look" || in.Seconds != 9 || in.MediaRef != nil || in.Download != nil {
		t.Fatalf("placeholder %+v", in)
	}
	if ci.GetStanzaID() != "Q1" {
		t.Fatalf("context %v", ci)
	}
}

func TestFieldNamesListsNoContent(t *testing.T) {
	m := &waE2E.Message{
		ViewOnceMessageV2:  &waE2E.FutureProofMessage{Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("secret")}}},
		MessageContextInfo: &waE2E.MessageContextInfo{},
	}
	got := fieldNames(m)
	if want := []string{"messageContextInfo", "viewOnceMessageV2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fields %v, want %v", got, want)
	}
}
