package docker

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
)

// mergeKey is the key that merges another mapping into the one it is in.
const mergeKey = "<<"

// Labelled is a stack's compose file with every one of its services
// labelled as the stack's: stack.LabelStack, with the stack's uuid, and
// stack.LabelServices, with the services that are to be running. Whatever
// compose makes of it then says which stack it belongs to, and what of the
// stack is to be running, to whoever reads the containers and keeps no record
// of the file.
//
// It is the file as it was written otherwise, comments and all. A service's
// own labels are kept, in the form they were written in, a map or a list,
// and a label of the stack's they had already is the stack's. Labels a
// service shares through an anchor, or takes from another mapping by a merge
// key, are copied into the service before the stack's are added, so that
// neither the anchor's other users nor the merged mapping change. A service
// that is not to be running is labelled all the same, and left out of
// stack.LabelServices: one behind a profile, one scaled to none, and one
// another service waits on to have completed, which runs once and ends.
func Labelled(compose string, stackUUID string) (string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(compose), &document); err != nil {
		return "", err
	}

	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return "", errors.New("it is empty")
	}

	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return "", errors.New("it is not a mapping")
	}

	services := resolved(lookup(root, "services"))
	if services == nil || services.Kind != yaml.MappingNode || len(services.Content) == 0 {
		return "", errors.New("it has no services")
	}

	labels := [][2]string{
		{stack.LabelStack, stackUUID},
		{stack.LabelServices, strings.Join(running(services), ",")},
	}

	for i := 0; i+1 < len(services.Content); i += 2 {
		name := services.Content[i].Value

		service, err := own(services.Content[i+1])
		if err != nil {
			return "", fmt.Errorf("service %q: %w", name, err)
		}

		services.Content[i+1] = service

		if err := label(service, labels); err != nil {
			return "", fmt.Errorf("service %q: %w", name, err)
		}
	}

	plainMerges(&document)

	var labelled bytes.Buffer

	encoder := yaml.NewEncoder(&labelled)
	encoder.SetIndent(2)

	if err := encoder.Encode(&document); err != nil {
		return "", err
	}

	if err := encoder.Close(); err != nil {
		return "", err
	}

	return labelled.String(), nil
}

// own is a service as a mapping of its own: one that is an alias is a copy
// of what it names, and one that says nothing is an empty mapping.
func own(service *yaml.Node) (*yaml.Node, error) {
	switch {
	case service.Kind == yaml.AliasNode && service.Alias != nil:
		service = copied(service.Alias)
	case isNull(service):
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	}

	if service.Kind != yaml.MappingNode {
		return nil, errors.New("it is not a mapping")
	}

	return service, nil
}

// label gives a service each label of labels, in whichever form its own
// labels are written in.
func label(service *yaml.Node, labels [][2]string) error {
	var node *yaml.Node

	if i := keyIndex(service, "labels"); i >= 0 {
		node = service.Content[i+1]
		if node.Kind == yaml.AliasNode && node.Alias != nil {
			node = copied(node.Alias)
			service.Content[i+1] = node
		}
	} else {
		// what a merge key brings in is given up by a key of the service's
		// own, so it is copied in first.
		node = copiedOrNil(lookup(service, "labels"))
		if node == nil {
			node = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}

		service.Content = append(service.Content, scalar("labels"), node)
	}

	if isNull(node) {
		*node = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}

	switch node.Kind {
	case yaml.MappingNode:
		for _, l := range labels {
			set(node, l[0], l[1])
		}
	case yaml.SequenceNode:
		for _, l := range labels {
			node.Content = slices.DeleteFunc(node.Content, func(item *yaml.Node) bool {
				return item.Kind == yaml.ScalarNode && (item.Value == l[0] || strings.HasPrefix(item.Value, l[0]+"="))
			})

			node.Content = append(node.Content, quoted(l[0]+"="+l[1]))
		}
	default:
		return errors.New("its labels are neither a map nor a list")
	}

	return nil
}

// running are the services that are to be running once the stack is up,
// by name, in order.
func running(services *yaml.Node) []string {
	// those another service waits on to have completed run once, and end.
	completed := make(map[string]bool)

	for i := 1; i < len(services.Content); i += 2 {
		dependsOn := resolved(lookup(resolved(services.Content[i]), "depends_on"))
		if dependsOn == nil || dependsOn.Kind != yaml.MappingNode {
			continue
		}

		for j := 0; j+1 < len(dependsOn.Content); j += 2 {
			condition := resolved(lookup(resolved(dependsOn.Content[j+1]), "condition"))
			if condition != nil && condition.Value == "service_completed_successfully" {
				completed[dependsOn.Content[j].Value] = true
			}
		}
	}

	var names []string

	for i := 0; i+1 < len(services.Content); i += 2 {
		name := services.Content[i].Value
		service := resolved(services.Content[i+1])

		profiles := resolved(lookup(service, "profiles"))
		scale := resolved(lookup(service, "scale"))
		replicas := resolved(lookup(resolved(lookup(service, "deploy")), "replicas"))

		switch {
		case completed[name]:
		case profiles != nil && profiles.Kind == yaml.SequenceNode && len(profiles.Content) > 0:
		case scale != nil && scale.Value == "0":
		case replicas != nil && replicas.Value == "0":
		default:
			names = append(names, name)
		}
	}

	slices.Sort(names)

	return names
}

// lookup is the value of key in a mapping: its own, or else the first of
// the mappings its merge key brings in that has one. Anything that is not a
// mapping has no keys.
func lookup(mapping *yaml.Node, key string) *yaml.Node {
	mapping = resolved(mapping)
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}

	if i := keyIndex(mapping, key); i >= 0 {
		return mapping.Content[i+1]
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != mergeKey {
			continue
		}

		merged := resolved(mapping.Content[i+1])

		sources := []*yaml.Node{merged}
		if merged != nil && merged.Kind == yaml.SequenceNode {
			sources = merged.Content
		}

		for _, source := range sources {
			if value := lookup(source, key); value != nil {
				return value
			}
		}
	}

	return nil
}

// keyIndex is where key is in a mapping's own keys, or -1.
func keyIndex(mapping *yaml.Node, key string) int {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Kind == yaml.ScalarNode && mapping.Content[i].Value == key {
			return i
		}
	}

	return -1
}

// set gives a mapping key with value, in place of what it had.
func set(mapping *yaml.Node, key string, value string) {
	if i := keyIndex(mapping, key); i >= 0 {
		mapping.Content[i+1] = quoted(value)

		return
	}

	mapping.Content = append(mapping.Content, scalar(key), quoted(value))
}

// resolved is what a node stands for: the node an alias names, or the node.
func resolved(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}

	return node
}

// copiedOrNil is a copy of what node stands for, or nil for nothing.
func copiedOrNil(node *yaml.Node) *yaml.Node {
	if node = resolved(node); node == nil {
		return nil
	}

	return copied(node)
}

// copied is a node of its own, the same as node and sharing nothing with it,
// but for the aliases inside it, which still name what they named. Nothing in
// it is an anchor: the anchors are still the nodes it was copied from, which
// stay where they are.
func copied(node *yaml.Node) *yaml.Node {
	c := *node
	c.Anchor = ""
	c.Content = make([]*yaml.Node, len(node.Content))

	for i, child := range node.Content {
		if child.Kind == yaml.AliasNode {
			alias := *child
			c.Content[i] = &alias

			continue
		}

		c.Content[i] = copied(child)
	}

	return &c
}

// plainMerges writes every merge key as it is usually written, `<<`, rather
// than with the tag it was read with, which is what it is read as all the
// same.
func plainMerges(node *yaml.Node) {
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if key := node.Content[i]; key.Kind == yaml.ScalarNode && key.Value == mergeKey && key.Tag == "!!merge" {
				key.Tag = ""
			}
		}
	}

	for _, child := range node.Content {
		plainMerges(child)
	}
}

// isNull reports whether node says nothing: `labels:` with nothing after it.
func isNull(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Tag == "!!null"
}

func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// quoted is a string that stays one, whatever it looks like.
func quoted(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value, Style: yaml.DoubleQuotedStyle}
}
