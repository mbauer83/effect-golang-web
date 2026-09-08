package unit

// The .proto file a service implies, compiled by a real protobuf compiler.
//
// This is the deliverable that makes gRPC usable from anywhere else: another
// language generates its client from this file, so a field number changing here
// changes that client's wire format. A golden string would say only that the
// projection emits what it emitted last week; what matters is that protoc
// accepts it and that the descriptor says what the description said.

import (
	"context"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/mbauer83/effect-golang-web/examples/quoting"
	"github.com/mbauer83/effect-golang-web/grpc"
)

func TestTheContractCompilesAndDeclaresTheService(t *testing.T) {
	document, err := grpc.Contract("logistics.v1", quoting.Quote)
	if err != nil {
		t.Fatal(err)
	}
	rendered := document.Render()

	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(
				map[string]string{"contract.proto": rendered}),
		}),
	}
	files, err := compiler.Compile(context.Background(), "contract.proto")
	if err != nil {
		t.Fatalf("the contract did not compile: %v\n%s", err, rendered)
	}

	services := files[0].Services()
	if services.Len() != 1 {
		t.Fatalf("expected one service, got %d:\n%s", services.Len(), rendered)
	}
	service := services.Get(0)
	// The fully-qualified name, because that is what forms the path a client
	// calls -- and the path this module answers at has to be the same one.
	if string(service.FullName()) != "logistics.v1.Rates" {
		t.Errorf("unexpected service: %s", service.FullName())
	}

	method := service.Methods().ByName("Quote")
	if method == nil {
		t.Fatalf("Quote is not in the descriptor:\n%s", rendered)
	}
	if string(method.Input().Name()) != "Enquiry" ||
		string(method.Output().Name()) != "Rate" {
		t.Errorf("unexpected shapes: %s -> %s", method.Input().Name(), method.Output().Name())
	}
	// Unary, which is what a request and a response describe. A streaming
	// procedure would need the description to say which side streams.
	if method.IsStreamingClient() || method.IsStreamingServer() {
		t.Error("expected a unary method")
	}

	// The path the descriptor implies is the path the transport answers at, so
	// a client generated from this file reaches the handler.
	if path := "/" + string(service.FullName()) + "/" + string(method.Name()); path != quoting.Quote.Path() {
		t.Errorf("the contract says %s and the transport answers %s", path, quoting.Quote.Path())
	}

	assertShapes(t, files[0].Messages())
}

// assertShapes checks the messages the service's shapes became.
func assertShapes(t *testing.T, messages protoreflect.MessageDescriptors) {
	t.Helper()
	found := map[string]protoreflect.MessageDescriptor{}
	for index := range messages.Len() {
		found[string(messages.Get(index).Name())] = messages.Get(index)
	}
	enquiry, held := found["Enquiry"]
	if !held {
		t.Fatal("Enquiry is not in the file")
	}
	if kilos := enquiry.Fields().ByName("kilos"); kilos == nil ||
		kilos.Number() != 3 || kilos.Kind() != protoreflect.DoubleKind {
		t.Errorf("unexpected kilos: %v", kilos)
	}
	if _, held := found["Rate"]; !held {
		t.Error("Rate is not in the file")
	}
}

func TestTheContractCarriesTheProseAndTheConstraints(t *testing.T) {
	document, err := grpc.Contract("logistics.v1", quoting.Quote)
	if err != nil {
		t.Fatal(err)
	}
	rendered := document.Render()

	for _, said := range []string{
		// The bare name, because the package declaration supplies the rest --
		// and the fully-qualified name the client calls is the two together.
		"service Rates {",
		"rpc Quote(Enquiry) returns (Rate);",
		"// Quote prices one shipment, or says why it cannot be priced.",
		"// Kilos is what it weighs, and it weighs something.",
		// Proto3 has no validation keywords, so what the schema enforces is
		// carried as prose: the reader of the contract needs to know it, and
		// the server does enforce it through the same schema.
		"// above 0",
		"// matching ^[A-Z]{3}$",
	} {
		if !strings.Contains(rendered, said) {
			t.Errorf("expected %q in the contract:\n%s", said, rendered)
		}
	}
}

func TestAShapeTwoProceduresShareIsDeclaredOnce(t *testing.T) {
	// The claim the projection makes: a type used by ten procedures appears
	// once. With one procedure it is untestable, so this is where it is tested.
	revised := grpc.Unary(quoting.Service, "Revise",
		quoting.EnquirySchema, quoting.RateSchema).
		Documented("Revise prices a shipment again.")

	document, err := grpc.Contract("logistics.v1", quoting.Quote, revised)
	if err != nil {
		t.Fatal(err)
	}
	rendered := document.Render()

	if declared := strings.Count(rendered, "message Enquiry {"); declared != 1 {
		t.Errorf("expected Enquiry declared once, got %d:\n%s", declared, rendered)
	}
	if declared := strings.Count(rendered, "message Rate {"); declared != 1 {
		t.Errorf("expected Rate declared once, got %d", declared)
	}
	// Both procedures in one service, because they name the same one -- and in
	// the order they were given, so the file can be checked in.
	if declared := strings.Count(rendered, "service Rates {"); declared != 1 {
		t.Errorf("expected one service, got %d", declared)
	}
	quoteAt := strings.Index(rendered, "rpc Quote(")
	reviseAt := strings.Index(rendered, "rpc Revise(")
	if quoteAt < 0 || reviseAt < 0 || quoteAt > reviseAt {
		t.Errorf("expected both procedures in the order given:\n%s", rendered)
	}

	// And it still compiles, which is the only reason to believe any of it.
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(
				map[string]string{"shared.proto": rendered}),
		}),
	}
	if _, err := compiler.Compile(context.Background(), "shared.proto"); err != nil {
		t.Fatalf("the shared contract did not compile: %v\n%s", err, rendered)
	}
}

func TestAContractNamingAServiceOutsideItsPackageIsRefused(t *testing.T) {
	// The file would compile and declare a service at an address nobody calls,
	// because the path a client builds is the package and the name together.
	elsewhere := grpc.Unary("shipping.v2.Rates", "Quote",
		quoting.EnquirySchema, quoting.RateSchema)

	_, err := grpc.Contract("logistics.v1", elsewhere)
	if err == nil {
		t.Fatal("expected the mismatch to be refused")
	}
	if !strings.Contains(err.Error(), "would not be the path this answers at") {
		t.Fatalf("expected the reason to say so, got %v", err)
	}
}
