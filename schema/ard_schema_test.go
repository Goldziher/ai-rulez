package schema

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The [ard] publisher and namespace patterns must agree with the URN grammar
// the ard package enforces (Appendix C of the ARD spec).
func TestARDTableMatchesTheURNGrammar(t *testing.T) {
	doc := func(publisher, namespace string) []byte {
		return []byte(fmt.Sprintf(`{"version":"5.0","name":"x","ard":{"publisher":%q,"namespace":%q}}`, publisher, namespace))
	}
	valid := [][2]string{{"example.com", "tools"}, {"Example.COM", "team:tools"}, {"a-b.example.co.uk", "v1.2_x-y"}}
	for _, v := range valid {
		assert.NoError(t, ValidateWithSchema(doc(v[0], v[1])), "%v", v)
	}
	invalid := [][2]string{
		{"localhost", "tools"}, {"-bad.example.com", "tools"}, {"example.com.", "tools"}, {"exa mple.com", "tools"},
		{"example.com", ""}, {"example.com", "a b"}, {"example.com", "a::b"}, {"example.com", "a:"},
	}
	for _, v := range invalid {
		assert.Error(t, ValidateWithSchema(doc(v[0], v[1])), "%v", v)
	}
	assert.Error(t, ValidateWithSchema([]byte(`{"version":"5.0","name":"x","ard":{"publisher":"example.com"}}`)))
	assert.Error(t, ValidateWithSchema([]byte(`{"version":"5.0","name":"x","ard":{"publisher":"example.com","namespace":"t","base_url":"http://example.com"}}`)))
	assert.NoError(t, ValidateWithSchema([]byte(`{"version":"5.0","name":"x","ard":{"publisher":"example.com","namespace":"t","base_url":"https://example.com/ard","plugin_type":"application/vnd.example+json"}}`)))
}
