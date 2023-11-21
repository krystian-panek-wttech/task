package taskfile

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"strings"
)

type Glob struct {
	Pattern string
	Negate  bool
}

func (s *Glob) DeepCopy() *Glob {
	if s == nil {
		return nil
	}

	return &Glob{
		Pattern: s.Pattern,
		Negate:  s.Negate,
	}
}

func (s *Glob) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {

	case yaml.ScalarNode:
		var glob string
		if err := node.Decode(&glob); err != nil {
			return err
		}
		if strings.HasPrefix(glob, "!") {
			s.Pattern = glob[1:]
			s.Negate = true
		} else {
			s.Pattern = glob
			s.Negate = false
		}
		return nil

	case yaml.MappingNode:
		var excludeNode struct {
			Exclude string
		}
		if err := node.Decode(&excludeNode); err != nil {
			return err
		}
		s.Pattern = excludeNode.Exclude
		s.Negate = true
		return nil
	}

	return fmt.Errorf("cannot unmarshal %s into glob", node.ShortTag())
}
