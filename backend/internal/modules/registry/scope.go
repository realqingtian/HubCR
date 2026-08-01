package registry

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"hubcr.io/hubcr/internal/modules/namespaces"
	"hubcr.io/hubcr/internal/modules/repositories"
)

var actionOrder = []Action{ActionPull, ActionPush, ActionDelete}

func ParseScopes(values []string) ([]Scope, error) {
	if len(values) > MaxScopes {
		return nil, fmt.Errorf("%w: too many scope values", ErrInvalidRequest)
	}
	merged := make(map[string]map[Action]struct{}, len(values))
	partsByName := make(map[string][2]string, len(values))
	for _, value := range values {
		scope, err := ParseScope(value)
		if err != nil {
			return nil, err
		}
		actions, exists := merged[scope.Name]
		if !exists {
			actions = make(map[Action]struct{}, len(scope.Actions))
			merged[scope.Name] = actions
			partsByName[scope.Name] = [2]string{scope.Namespace, scope.Repository}
		}
		for _, action := range scope.Actions {
			actions[action] = struct{}{}
		}
	}

	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]Scope, 0, len(names))
	for _, name := range names {
		parts := partsByName[name]
		result = append(result, Scope{
			Type: ResourceRepository, Name: name,
			Namespace: parts[0], Repository: parts[1],
			Actions: orderedActions(merged[name]),
		})
	}
	return result, nil
}

func ParseScope(value string) (Scope, error) {
	if value == "" || len(value) > MaxScopeBytes || !utf8.ValidString(value) {
		return Scope{}, fmt.Errorf("%w: malformed scope", ErrInvalidRequest)
	}
	first := strings.IndexByte(value, ':')
	last := strings.LastIndexByte(value, ':')
	if first <= 0 || last <= first+1 || last == len(value)-1 {
		return Scope{}, fmt.Errorf("%w: malformed scope", ErrInvalidRequest)
	}
	resourceType := value[:first]
	name := value[first+1 : last]
	rawActions := value[last+1:]
	if resourceType != ResourceRepository {
		return Scope{}, fmt.Errorf("%w: unsupported resource type", ErrInvalidRequest)
	}
	nameParts := strings.Split(name, "/")
	if len(nameParts) != 2 {
		return Scope{}, fmt.Errorf("%w: malformed repository name", ErrInvalidRequest)
	}
	namespace, err := namespaces.NormalizeName(nameParts[0])
	if err != nil || namespace != nameParts[0] {
		return Scope{}, fmt.Errorf("%w: malformed namespace", ErrInvalidRequest)
	}
	repository, err := repositories.NormalizeName(nameParts[1])
	if err != nil || repository != nameParts[1] {
		return Scope{}, fmt.Errorf("%w: malformed repository", ErrInvalidRequest)
	}

	actionSet := make(map[Action]struct{}, 3)
	for _, rawAction := range strings.Split(rawActions, ",") {
		action := Action(rawAction)
		switch action {
		case ActionPull, ActionPush, ActionDelete:
			actionSet[action] = struct{}{}
		default:
			return Scope{}, fmt.Errorf("%w: unsupported repository action", ErrInvalidRequest)
		}
	}
	return Scope{
		Type: ResourceRepository, Name: namespace + "/" + repository,
		Namespace: namespace, Repository: repository,
		Actions: orderedActions(actionSet),
	}, nil
}

func orderedActions(set map[Action]struct{}) []Action {
	result := make([]Action, 0, len(set))
	for _, action := range actionOrder {
		if _, exists := set[action]; exists {
			result = append(result, action)
		}
	}
	return result
}
