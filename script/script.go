package script

import (
	"encoding/json"
	"fmt"
	"github.com/mailru/easyjson"
	configV1 "github.com/nlnwa/veidemann-api/go/config/v1"
)

// ReturnValue is the return value format for scripts of type
// ON_LOAD, ON_NEW_DOCUMENT and UNDEFINED.
type ReturnValue struct {
	// WaitForData specifies whether or not one should wait for
	// network activity to settle after script execution.
	WaitForData bool `json:"waitForData,omitempty"`
	// Next specifies the ID of a script to be executed next.
	Next string `json:"next,omitempty"`
	// Data specifies arguments to a potential next script.
	Data easyjson.RawMessage `json:"data,omitempty"`
}

// String implements the Stringer interface
func (rv ReturnValue) String() string {
	str, err := json.Marshal(rv)
	if err != nil {
		return ""
	}
	return string(str)
}

// Run runs the script with key scriptId in the given scripts map.
//
// Depending on the id in the Next field of the return value any other script
// in the map may be executed sequentially.
//
// The script arguments are compiled from current script annotations, seed
// annotations and the contents of the data object returned by the previously
// executed script (if any). Seed annotations has higher precedence than script
// annotations and the data field from the return value have highest precedence.
func Run(
	next string,
	scripts map[string]*configV1.ConfigObject,
	annotations []*configV1.Annotation,
	execute func(*configV1.ConfigObject, easyjson.RawMessage) (easyjson.RawMessage, error),
	wait func(),
) error {
	var data map[string]interface{}

	for len(next) > 0 {
		script, ok := scripts[next]
		if !ok {
			return fmt.Errorf("failed to find script with id: %s", next)
		}
		name := script.GetMeta().GetName()

		params := make(map[string]interface{})
		for _, blueprint := range script.GetMeta().GetAnnotation() {
			for _, annotation := range annotations {
				if annotation.Key == blueprint.Key {
					params[annotation.Key] = annotation.Value
				}
			}
		}
		// add data from return value of previous script as arguments to next script
		for key, value := range data {
			params[key] = value
		}

		arguments, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal script arguments for script %s (%s): %w", name, next, err)
		}

		res, err := execute(script, arguments)
		if err != nil {
			return fmt.Errorf("failed to execute script %s (%s): %w", name, next, err)
		}
		if res == nil {
			return nil
		}
		var rv ReturnValue
		err = json.Unmarshal(res, &rv)
		if err != nil {
			return fmt.Errorf("failed to unmarshal result from script %s (%s): %w", name, next, err)
		}
		if rv.Next == "self" {
			next = script.Id
		} else {
			next = rv.Next
		}
		if len(rv.Data) > 0 {
			err := json.Unmarshal(rv.Data, &data)
			if err != nil {
				return fmt.Errorf("failed to unmarshal data from script %s (%s): %w", name, next, err)
			}
		} else {
			data = make(map[string]interface{})
		}
		if rv.WaitForData {
			wait()
		}
	}
	return nil
}
