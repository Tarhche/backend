// Package certificate makes the certificates the runner's tunnel authenticates
// with: one authority, one certificate for the ingress, and one for each
// orchestrator.
//
//	app certificate authority generate --output-dir ./certs/ca
//	app certificate ingress   generate --output-dir ./certs/ingress   --name ingress.example.internal
//	app certificate orchestrator    generate --output-dir ./certs/orchestrator-001 --name orchestrator-001
//
// A private key is written to disk and never anywhere else. Nothing here prints
// one, and nothing here sends one.
package certificate

import (
	"github.com/danceable/console"
)

// Group builds the command group that makes certificates.
func Group() *console.Group {
	authority := console.NewGroup("authority", "makes the authority that signs the rest.").Register(NewAuthorityCommand())
	ingress := console.NewGroup("ingress", "makes the certificate an ingress answers with.").Register(NewIngressCommand())
	orchestrator := console.NewGroup("orchestrator", "makes the certificate an orchestrator proves itself with.").Register(NewOrchestratorCommand())

	return console.NewGroup("certificate", "makes the certificates the tunnel authenticates with.").RegisterGroup(authority, ingress, orchestrator)
}
