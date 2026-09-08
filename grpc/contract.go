package grpc

// The .proto file a set of procedures implies.
//
// It is the same idea as the OpenAPI document a surface implies: the contract
// is a projection of descriptions already held, so it cannot drift from what
// the server actually accepts. What it is *for* is different, though, and worth
// saying: an OpenAPI document is read, where a .proto file is compiled --
// another language generates its client from this, so a field number changing
// here changes that client's wire format.

import (
	"fmt"
	"strings"

	"github.com/mbauer83/effect-golang-schema/schema/protobuf"
	"github.com/mbauer83/effect-golang-schema/schema/structure"
)

// Declaring is a procedure a contract can be projected from.
//
// An interface rather than a generic parameter because a contract covers
// several procedures whose types differ, and there is nothing to gain from
// naming them: what a projection needs is the name and the two descriptions.
type Declaring interface {
	Service() string
	Method() string
	Doc() string
	Shapes() (request, response structure.Node)
	Fault() error
}

// Contract is the proto3 file these procedures imply.
//
// Every shape is declared once and shared, so a type used by ten procedures
// appears once -- and the order is the order the procedures were given, so the
// same set always produces the same file and the file can be checked in.
func Contract(packageName string, procedures ...Declaring) (protobuf.Document, error) {
	declared := make([]protobuf.Declared, 0, len(procedures))
	for _, procedure := range procedures {
		if err := procedure.Fault(); err != nil {
			return protobuf.Document{}, err
		}
		bare, err := within(packageName, procedure.Service())
		if err != nil {
			return protobuf.Document{}, err
		}
		request, response := procedure.Shapes()
		declared = append(declared, protobuf.Declared{
			Service:  bare,
			Method:   procedure.Method(),
			Doc:      procedure.Doc(),
			Request:  request,
			Response: response,
		})
	}
	return protobuf.ProjectServices(packageName, declared...)
}

// within is the service's own name inside the package that declares it.
//
// A procedure names its service in full, because the full name is what forms
// the path a client calls. A proto file names it bare, because the package
// declaration supplies the rest. So the two have to agree, and a service whose
// name is not inside the package being projected is a mistake worth naming: the
// file would compile and declare a service at an address nobody calls.
func within(packageName string, service string) (string, error) {
	prefix := packageName + "."
	if !strings.HasPrefix(service, prefix) {
		return "", faulted("projecting a contract", service,
			fmt.Errorf("%q is not in package %q, so the path a client calls would not be the "+
				"path this answers at", service, packageName))
	}
	bare := strings.TrimPrefix(service, prefix)
	if bare == "" || strings.Contains(bare, ".") {
		return "", faulted("projecting a contract", service,
			fmt.Errorf("%q names a service inside a nested package, and a file declares one",
				service))
	}
	return bare, nil
}
