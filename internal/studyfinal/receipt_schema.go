package studyfinal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	contextartifact "github.com/Siddhant-K-code/distill/research/context-is-a-build-artifact"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	finalReceiptSchemaOnce sync.Once
	finalReceiptSchema     *jsonschema.Schema
	finalReceiptSchemaErr  error
)

func validateReceiptSchema(receipt Receipt) error {
	finalReceiptSchemaOnce.Do(func() {
		sum := sha256.Sum256(contextartifact.FinalReceiptSchema)
		if hex.EncodeToString(sum[:]) != contextartifact.FinalReceiptSchemaSHA256 {
			finalReceiptSchemaErr = fmt.Errorf("embedded final receipt schema raw digest mismatch")
			return
		}
		var document any
		if err := json.Unmarshal(contextartifact.FinalReceiptSchema, &document); err != nil {
			finalReceiptSchemaErr = fmt.Errorf("decode final receipt schema: %w", err)
			return
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		compiler.AssertFormat()
		if err := compiler.AddResource("urn:distill:context-build-artifact:final-receipt:v1", document); err != nil {
			finalReceiptSchemaErr = fmt.Errorf("load final receipt schema: %w", err)
			return
		}
		finalReceiptSchema, finalReceiptSchemaErr = compiler.Compile("urn:distill:context-build-artifact:final-receipt:v1")
	})
	if finalReceiptSchemaErr != nil {
		return finalReceiptSchemaErr
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if err := finalReceiptSchema.Validate(value); err != nil {
		return fmt.Errorf("final receipt schema validation: %w", err)
	}
	return nil
}
