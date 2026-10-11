package stack

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/khanzadimahdi/testproject/domain"
)

// TestShapes holds the keys reading.shaped holds each mapping of a stack's
// file whose keys are fixed to, to the documents the file is decoded into
// strictly (document), which yaml.Decoder.KnownFields holds it to: every type
// of such a mapping that document reaches is paired with keys, which are its
// fields' as their yaml tags name them, 64 at most; and wherever a mapping of
// it is in a file, shaped refuses a key that is none of them under its own
// path, before the strict decode would refuse it under the file's, and takes
// each of them. A field a document is given, or a type of mapping it reaches
// that shaped holds to no keys, fails it until shaped is.
func TestShapes(t *testing.T) {
	t.Parallel()

	paired := map[reflect.Type][]string{
		reflect.TypeFor[document]():          fileKeys,
		reflect.TypeFor[serviceDocument]():   serviceKeys,
		reflect.TypeFor[resourcesDocument](): resourcesKeys,
		reflect.TypeFor[limitsDocument]():    limitsKeys,
	}

	places := shapesOf(reflect.TypeFor[document]())

	for shape, steps := range places {
		t.Run(shape.Name(), func(t *testing.T) {
			t.Parallel()

			keys, isPaired := paired[shape]
			require.True(t, isPaired, "a %s is a mapping of a stack's file whose keys are fixed, which shaped holds to no keys", shape)

			assert.ElementsMatch(t, tagsOf(shape), keys, "shaped holds a %s to the keys its fields are", shape)
			assert.LessOrEqual(t, len(keys), 64, "and tells each of them given twice of 64 at most")

			_, invalid := Parse(within(steps, "{bogus: 1}"))
			assert.Equal(t, CodeUnsupportedKey, invalid[Field(pathOf(steps, "bogus"))], "a key none of them is refused under its own path: %v", invalid)

			for _, key := range keys {
				_, invalid := Parse(within(steps, "{"+key+": ~}"))
				assert.NotEqual(t, CodeUnsupportedKey, invalid[Field(pathOf(steps, key))], "and %s is taken: %v", key, invalid)
			}
		})
	}

	for shape := range paired {
		assert.Contains(t, places, shape, "a %s is a mapping of a stack's file", shape)
	}
}

// shapesOf is each type of a mapping whose keys are fixed that a mapping of
// the type of top reaches, and the steps of the path to where it is in a
// file, from the file's top: a field by its yaml tag, a name of a mapping of
// names as x, and an item of a sequence as [0]. What a node takes, a value
// kept as it is written, is none.
func shapesOf(top reflect.Type) map[reflect.Type][]string {
	places := make(map[reflect.Type][]string)

	var reach func(t reflect.Type, steps []string)
	reach = func(t reflect.Type, steps []string) {
		switch t.Kind() {
		case reflect.Pointer:
			reach(t.Elem(), steps)
		case reflect.Map:
			reach(t.Elem(), append(slices.Clone(steps), "x"))
		case reflect.Slice, reflect.Array:
			reach(t.Elem(), append(slices.Clone(steps), "[0]"))
		case reflect.Struct:
			if _, reached := places[t]; reached || t == reflect.TypeFor[yaml.Node]() {
				return
			}

			places[t] = steps

			for i := range t.NumField() {
				reach(t.Field(i).Type, append(slices.Clone(steps), tagOf(t.Field(i))))
			}
		}
	}

	reach(top, nil)

	return places
}

// tagsOf are the keys a mapping decoded into a t has, as its fields' yaml
// tags name them.
func tagsOf(t reflect.Type) []string {
	tags := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		tags = append(tags, tagOf(t.Field(i)))
	}

	return tags
}

// tagOf is the key a field is decoded from.
func tagOf(f reflect.StructField) string {
	key, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")

	return key
}

// within is a file of nothing but inner, beneath steps from its top.
func within(steps []string, inner string) string {
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i] == "[0]" {
			inner = "[" + inner + "]"
		} else {
			inner = "{" + steps[i] + ": " + inner + "}"
		}
	}

	return inner
}

// pathOf is the path of key beneath steps from the top of a file.
func pathOf(steps []string, key string) string {
	path := ""
	for _, step := range steps {
		if step == "[0]" {
			path = indexed(path, 0)
		} else {
			path = field(path, step)
		}
	}

	return field(path, key)
}

func TestParse_strictly(t *testing.T) {
	t.Parallel()

	t.Run("a file shaped as the platform reads one is decoded strictly too, which a key that is no field of the platform's shape fails", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, strictly("services:\n  web:\n    image: nginx\n    resources: {limits: {cpus: 1}}\nvolumes: {data: ~}\nnetworks: {front: {}}\n"))

		for _, file := range []string{
			"services:\n  web:\n    image: nginx\n    healthcheck: {test: x}\n",
			"services:\n  web:\n    image: nginx\n    resources: {limits: {pids: 1}}\n",
			"services:\n  web:\n    image: nginx\nname: shop\n",
		} {
			assert.Error(t, strictly(file), file)
		}
	})

	t.Run("and one the strict decode refuses though the reading took it, a mistake of the reading's, is refused under the file's own path", func(t *testing.T) {
		t.Parallel()

		file, invalid := parse("services:\n  web:\n    image: nginx\n", func(string) error {
			return errors.New("yaml: unmarshal errors:\n  line 3: field bogus not found in type stack.serviceDocument")
		})

		assert.Equal(t, domain.ValidationErrors{"compose": "invalid_value"}, invalid)
		assert.Empty(t, file.Services, "and is no file")
	})

	t.Run("but one it takes is read", func(t *testing.T) {
		t.Parallel()

		file, invalid := parse("services:\n  web:\n    image: nginx\n", strictly)
		require.Empty(t, invalid)

		assert.Equal(t, "nginx", file.Services["web"].Image)
	})
}
