package unit

// A surface's naming strategy. What matters is that one declaration spells the
// members of everything the surface reads, writes and publishes, and that a
// document spelled another way is refused as a client's mistake rather than
// read by guesswork.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-schema/schema"
	spelling "github.com/mbauer83/effect-golang-schema/schema/naming"
	"github.com/mbauer83/effect-golang-schema/schema/structure"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// note has members of two words, so a strategy has something to respell.
type note struct {
	BodyText  string
	CreatedAt string
}

var noteSchema = schema.Struct[note]("note",
	schema.FieldOf("body_text", schema.Text(),
		func(value note) string { return value.BodyText },
		func(value *note, text string) { value.BodyText = text }),
	schema.FieldOf("created_at", schema.Text(),
		func(value note) string { return value.CreatedAt },
		func(value *note, at string) { value.CreatedAt = at }),
)

// camelSurface echoes a note, spelling its documents in camelCase.
func camelSurface(t *testing.T) web.Routes[effect.Unit, Refusal] {
	t.Helper()
	echoNote := web.Handle(
		web.Declare(http.MethodPost, "/notes", web.Entity(noteSchema), web.Returns(http.StatusOK, noteSchema)),
		func(given note) webEffect[note] { return effect.For[effect.Unit, Refusal]().Succeed(given) },
	)
	surface, err := web.NewRoutes(echoNote)
	if err != nil {
		t.Fatal(err)
	}
	return surface.WithNaming(spelling.CamelCase)
}

func sentTo(t *testing.T, surface web.Routes[effect.Unit, Refusal], request *http.Request) *http.Response {
	t.Helper()
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	boundary.Handler(surface.Handler()).ServeHTTP(recorder, request)
	return recorder.Result()
}

func TestASurfaceReadsAndWritesItsDocumentsSpelledByItsStrategy(t *testing.T) {
	received := sentTo(t, camelSurface(t), httptest.NewRequest(http.MethodPost, "/notes",
		strings.NewReader(`{"bodyText":"hello","createdAt":"today"}`)))

	if received.StatusCode != http.StatusOK {
		t.Fatalf("expected the camelCase entity read, got %d: %s", received.StatusCode, bodyOf(t, received))
	}
	if body := answered(t, received); body != `{"bodyText":"hello","createdAt":"today"}` {
		t.Fatalf("expected the answer in camelCase, got %s", body)
	}
}

func TestASurfaceRefusesADocumentSpelledAnotherWay(t *testing.T) {
	received := sentTo(t, camelSurface(t), httptest.NewRequest(http.MethodPost, "/notes",
		strings.NewReader(`{"body_text":"hello","created_at":"today"}`)))

	if received.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected the snake_case entity refused, got %d", received.StatusCode)
	}
}

func TestASurfaceDeclaresItsDocumentsAsItSpellsThem(t *testing.T) {
	declaration := camelSurface(t).Declarations()[0]
	for which, content := range map[string]*web.Content{"entity": declaration.Entity, "answer": declaration.Content} {
		object := content.Node.(structure.Object)
		if object.Fields[0].Name != "bodyText" || object.Fields[1].Name != "createdAt" {
			t.Errorf("expected the %s declared in camelCase, got %+v", which, object.Fields)
		}
	}
}
