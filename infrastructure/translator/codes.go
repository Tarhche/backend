package translator

import contract "github.com/khanzadimahdi/testproject/domain/translator"

// Codes answers every key with the key itself.
//
// It is for a service that is not the one to put a code into words: the
// workload's control plane answers the blog, which knows which language the
// person asking reads and translates what the control plane refused into it.
// Translated on the way, the code would arrive as an English sentence that no
// longer says which code it was.
type Codes struct{}

var _ contract.Translator = Codes{}

func (Codes) Translate(key string, _ ...func(*contract.Params)) string {
	return key
}
