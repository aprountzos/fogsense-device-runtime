package spec

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	NameRegex = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)
	TypeRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,40}$`)
)

const (
	MaxTelemetry  = 8
	MaxState      = 8
	MaxCommands   = 10
	MaxParams     = 5
	MaxEnumValues = 16
)

type FollowsSpec struct {
	State string                 `json:"state" yaml:"state"`
	When  map[string]interface{} `json:"when" yaml:"when"`
	Rate  float64                `json:"rate" yaml:"rate"`
}

type MetricSpec struct {
	Name        string       `json:"name" yaml:"name"`
	Unit        string       `json:"unit" yaml:"unit"`
	Min         float64      `json:"min" yaml:"min"`
	Max         float64      `json:"max" yaml:"max"`
	Noise       float64      `json:"noise" yaml:"noise"`
	AnomalyRate float64      `json:"anomalyRate" yaml:"anomalyRate"`
	IntervalSec float64      `json:"intervalSec" yaml:"intervalSec"`
	Follows     *FollowsSpec `json:"follows,omitempty" yaml:"follows,omitempty"`
}

type TypeDef struct {
	Type    string      `json:"type" yaml:"type"`
	Values  []string    `json:"values,omitempty" yaml:"values,omitempty"`
	Min     *float64    `json:"min,omitempty" yaml:"min,omitempty"`
	Max     *float64    `json:"max,omitempty" yaml:"max,omitempty"`
	Initial interface{} `json:"initial,omitempty" yaml:"initial,omitempty"`
}

type StateFieldSpec struct {
	Name    string      `json:"name" yaml:"name"`
	Type    string      `json:"type" yaml:"type"`
	Values  []string    `json:"values,omitempty" yaml:"values,omitempty"`
	Min     *float64    `json:"min,omitempty" yaml:"min,omitempty"`
	Max     *float64    `json:"max,omitempty" yaml:"max,omitempty"`
	Initial interface{} `json:"initial" yaml:"initial"`
}

type CommandSpec struct {
	Name             string                 `json:"name" yaml:"name"`
	Params           map[string]TypeDef     `json:"params" yaml:"params"`
	Effects          map[string]interface{} `json:"effects" yaml:"effects"`
	UserControllable bool                   `json:"userControllable" yaml:"userControllable"`
}

type DeviceSpec struct {
	Type        string           `json:"type" yaml:"type"`
	Version     int              `json:"version" yaml:"version"`
	Description string           `json:"description" yaml:"description"`
	OfflineRate float64          `json:"offlineRate" yaml:"offlineRate"`
	Telemetry   []MetricSpec     `json:"telemetry" yaml:"telemetry"`
	State       []StateFieldSpec `json:"state" yaml:"state"`
	Commands    []CommandSpec    `json:"commands" yaml:"commands"`
}

type SpecError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type ValidationResult struct {
	OK     bool        `json:"ok"`
	Errors []SpecError `json:"errors"`
	Spec   *DeviceSpec `json:"spec"`
}

func ValueFits(t TypeDef, val interface{}) bool {
	switch t.Type {
	case "enum":
		str, ok := val.(string)
		if !ok {
			return false
		}
		for _, v := range t.Values {
			if v == str {
				return true
			}
		}
		return false
	case "number":
		num, ok := toFloat(val)
		if !ok {
			return false
		}
		if t.Min == nil || t.Max == nil {
			return false
		}
		return num >= *t.Min && num <= *t.Max
	case "boolean":
		_, ok := val.(bool)
		return ok
	default:
		return false
	}
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func checkUnknownKeys(m map[string]interface{}, allowed []string, path string, addErr func(string, string)) {
	allowedSet := make(map[string]bool)
	for _, a := range allowed {
		allowedSet[a] = true
	}
	for k := range m {
		if !allowedSet[k] {
			p := k
			if path != "" {
				p = path + "." + k
			}
			addErr(p, fmt.Sprintf("unknown field '%s'", k))
		}
	}
}

func parseTypeDef(def map[string]interface{}, path string, addErr func(string, string), withInitial bool) *TypeDef {
	tVal, ok := def["type"].(string)
	if !ok || (tVal != "enum" && tVal != "number" && tVal != "boolean") {
		addErr(path+".type", "type must be one of enum, number, boolean")
		return nil
	}

	out := &TypeDef{Type: tVal}
	switch tVal {
	case "enum":
		rawVals, ok := def["values"].([]interface{})
		if !ok || len(rawVals) == 0 || len(rawVals) > MaxEnumValues {
			addErr(path+".values", fmt.Sprintf("enum requires between 1 and %d values", MaxEnumValues))
			return nil
		}
		seen := make(map[string]bool)
		var values []string
		bad := false
		for i, rv := range rawVals {
			str, ok := rv.(string)
			if !ok || str == "" || strings.HasPrefix(str, "$") {
				addErr(fmt.Sprintf("%s.values[%d]", path, i), "enum values must be non-empty strings that do not start with '$'")
				bad = true
			} else if seen[str] {
				addErr(fmt.Sprintf("%s.values[%d]", path, i), fmt.Sprintf("duplicate enum value '%s'", str))
				bad = true
			}
			seen[str] = true
			values = append(values, str)
		}
		if bad {
			return nil
		}
		out.Values = values
	case "number":
		minVal, hasMin := toFloat(def["min"])
		maxVal, hasMax := toFloat(def["max"])
		if !hasMin || !hasMax {
			addErr(path, "number requires numeric min and max")
			return nil
		}
		if minVal >= maxVal {
			addErr(path, "min must be less than max")
			return nil
		}
		out.Min = &minVal
		out.Max = &maxVal
	}

	if withInitial {
		initVal, hasInit := def["initial"]
		if !hasInit {
			switch tVal {
			case "enum":
				initVal = out.Values[0]
			case "number":
				initVal = *out.Min
			default:
				initVal = false
			}
		}
		if !ValueFits(*out, initVal) {
			addErr(path+".initial", "initial value does not fit the declared type/range")
			return nil
		}
		out.Initial = initVal
	}

	return out
}

func normalizeYAML(val interface{}) interface{} {
	switch v := val.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(v))
		for k, val2 := range v {
			m[fmt.Sprint(k)] = normalizeYAML(val2)
		}
		return m
	case map[string]interface{}:
		m := make(map[string]interface{}, len(v))
		for k, val2 := range v {
			m[k] = normalizeYAML(val2)
		}
		return m
	case []interface{}:
		s := make([]interface{}, len(v))
		for i, val2 := range v {
			s[i] = normalizeYAML(val2)
		}
		return s
	default:
		return v
	}
}

func ValidateSpec(raw []byte) ValidationResult {
	var input interface{}
	if err := yaml.Unmarshal(raw, &input); err != nil {
		return ValidationResult{
			OK: false,
			Errors: []SpecError{
				{Path: "", Message: "spec must be valid YAML: " + err.Error()},
			},
		}
	}

	input = normalizeYAML(input)
	m, ok := input.(map[string]interface{})
	if !ok {
		return ValidationResult{
			OK: false,
			Errors: []SpecError{
				{Path: "", Message: "spec must be an object"},
			},
		}
	}

	var errors []SpecError
	addErr := func(p, msg string) {
		errors = append(errors, SpecError{Path: p, Message: msg})
	}

	checkUnknownKeys(m, []string{"type", "version", "description", "offlineRate", "telemetry", "state", "commands"}, "", addErr)

	typeStr, ok := m["type"].(string)
	if !ok || !TypeRegex.MatchString(typeStr) {
		addErr("type", "type must match ^[a-z0-9][a-z0-9-]{1,40}$")
	}

	var version int
	if vNum, ok := toFloat(m["version"]); !ok || vNum < 1 || vNum != float64(int(vNum)) {
		addErr("version", "version must be an integer >= 1")
	} else {
		version = int(vNum)
	}

	description := ""
	if descVal, exists := m["description"]; exists {
		if ds, ok := descVal.(string); ok {
			description = ds
		} else {
			addErr("description", "must be a string")
		}
	}

	offlineRate := 0.0
	if orVal, exists := m["offlineRate"]; exists {
		if rate, ok := toFloat(orVal); !ok || rate < 0 || rate > 1 {
			addErr("offlineRate", "must be a number between 0 and 1")
		} else {
			offlineRate = rate
		}
	}

	getArray := func(key string, max int) []interface{} {
		val, exists := m[key]
		if !exists {
			return nil
		}
		arr, ok := val.([]interface{})
		if !ok {
			addErr(key, "must be an array")
			return nil
		}
		if len(arr) > max {
			addErr(key, fmt.Sprintf("at most %d entries are allowed", max))
			return arr[:max]
		}
		return arr
	}

	telemetryIn := getArray("telemetry", MaxTelemetry)
	stateIn := getArray("state", MaxState)
	commandsIn := getArray("commands", MaxCommands)

	// Validate state first
	var state []StateFieldSpec
	stateByName := make(map[string]StateFieldSpec)

	for i, sVal := range stateIn {
		p := fmt.Sprintf("state[%d]", i)
		sMap, ok := sVal.(map[string]interface{})
		if !ok {
			addErr(p, "must be an object")
			continue
		}
		checkUnknownKeys(sMap, []string{"name", "type", "values", "min", "max", "initial"}, p, addErr)

		name, ok := sMap["name"].(string)
		if !ok || !NameRegex.MatchString(name) {
			addErr(p+".name", "invalid name (letters/digits/underscore, starts with a letter, max 32 chars)")
			continue
		}
		if _, exists := stateByName[name]; exists {
			addErr(p+".name", fmt.Sprintf("duplicate state field '%s'", name))
			continue
		}

		tDef := parseTypeDef(sMap, p, addErr, true)
		if tDef == nil {
			continue
		}

		sf := StateFieldSpec{
			Name:    name,
			Type:    tDef.Type,
			Values:  tDef.Values,
			Min:     tDef.Min,
			Max:     tDef.Max,
			Initial: tDef.Initial,
		}
		state = append(state, sf)
		stateByName[name] = sf
	}

	// Validate telemetry
	var telemetry []MetricSpec
	seenMetrics := make(map[string]bool)

	for i, mVal := range telemetryIn {
		p := fmt.Sprintf("telemetry[%d]", i)
		mMap, ok := mVal.(map[string]interface{})
		if !ok {
			addErr(p, "must be an object")
			continue
		}
		checkUnknownKeys(mMap, []string{"name", "unit", "min", "max", "noise", "anomalyRate", "intervalSec", "follows"}, p, addErr)

		name, ok := mMap["name"].(string)
		if !ok || !NameRegex.MatchString(name) {
			addErr(p+".name", "invalid name (letters/digits/underscore, starts with a letter, max 32 chars)")
			continue
		}
		if seenMetrics[name] {
			addErr(p+".name", fmt.Sprintf("duplicate metric '%s'", name))
			continue
		}
		seenMetrics[name] = true

		metricOK := true
		unit := ""
		if uVal, exists := mMap["unit"]; exists {
			if uStr, ok := uVal.(string); ok {
				unit = uStr
			} else {
				addErr(p+".unit", "must be a string")
				metricOK = false
			}
		}

		minVal, hasMin := toFloat(mMap["min"])
		maxVal, hasMax := toFloat(mMap["max"])
		if !hasMin || !hasMax {
			addErr(p, "min and max are required numbers")
			metricOK = false
		} else if minVal >= maxVal {
			addErr(p, "min must be less than max")
			metricOK = false
		}

		noise := 0.0
		if nVal, exists := mMap["noise"]; exists {
			if nNum, ok := toFloat(nVal); !ok || nNum < 0 {
				addErr(p+".noise", "must be a number >= 0")
				metricOK = false
			} else {
				noise = nNum
			}
		}

		anomalyRate := 0.0
		if arVal, exists := mMap["anomalyRate"]; exists {
			if arNum, ok := toFloat(arVal); !ok || arNum < 0 || arNum > 1 {
				addErr(p+".anomalyRate", "must be a number between 0 and 1")
				metricOK = false
			} else {
				anomalyRate = arNum
			}
		}

		intervalSec := 5.0
		if isVal, exists := mMap["intervalSec"]; exists {
			if isNum, ok := toFloat(isVal); !ok || isNum < 1 || isNum > 3600 {
				addErr(p+".intervalSec", "must be a number between 1 and 3600")
				metricOK = false
			} else {
				intervalSec = isNum
			}
		}

		var follows *FollowsSpec
		if fVal, exists := mMap["follows"]; exists {
			fp := p + ".follows"
			fMap, ok := fVal.(map[string]interface{})
			if !ok {
				addErr(fp, "must be an object")
				metricOK = false
			} else {
				checkUnknownKeys(fMap, []string{"state", "when", "rate"}, fp, addErr)
				stateName, _ := fMap["state"].(string)
				sf, sfExists := stateByName[stateName]
				if !sfExists || sf.Type != "number" {
					addErr(fp+".state", "must name an existing numeric state field")
					metricOK = false
				}

				whenMap := make(map[string]interface{})
				if whenRaw, exists := fMap["when"]; exists {
					wMap, ok := whenRaw.(map[string]interface{})
					if !ok {
						addErr(fp+".when", "must be an object")
						metricOK = false
					} else {
						for k, v := range wMap {
							targetSF, exists := stateByName[k]
							if !exists {
								addErr(fmt.Sprintf("%s.when.%s", fp, k), "unknown state field")
								metricOK = false
							} else {
								tDef := TypeDef{Type: targetSF.Type, Values: targetSF.Values, Min: targetSF.Min, Max: targetSF.Max}
								if !ValueFits(tDef, v) {
									addErr(fmt.Sprintf("%s.when.%s", fp, k), "value does not fit the state field")
									metricOK = false
								} else {
									whenMap[k] = v
								}
							}
						}
					}
				}

				rate := 0.15
				if rVal, exists := fMap["rate"]; exists {
					if rNum, ok := toFloat(rVal); !ok || rNum <= 0 || rNum > 1 {
						addErr(fp+".rate", "rate must be a number in (0, 1]")
						metricOK = false
					} else {
						rate = rNum
					}
				}

				if metricOK {
					follows = &FollowsSpec{State: stateName, When: whenMap, Rate: rate}
				}
			}
		}

		if metricOK {
			telemetry = append(telemetry, MetricSpec{
				Name:        name,
				Unit:        unit,
				Min:         minVal,
				Max:         maxVal,
				Noise:       noise,
				AnomalyRate: anomalyRate,
				IntervalSec: intervalSec,
				Follows:     follows,
			})
		}
	}

	// Validate commands
	var commands []CommandSpec
	seenCommands := make(map[string]bool)

	for i, cVal := range commandsIn {
		p := fmt.Sprintf("commands[%d]", i)
		cMap, ok := cVal.(map[string]interface{})
		if !ok {
			addErr(p, "must be an object")
			continue
		}
		checkUnknownKeys(cMap, []string{"name", "params", "effects", "userControllable"}, p, addErr)

		name, ok := cMap["name"].(string)
		if !ok || !NameRegex.MatchString(name) {
			addErr(p+".name", "invalid name (letters/digits/underscore, starts with a letter, max 32 chars)")
			continue
		}
		if seenCommands[name] {
			addErr(p+".name", fmt.Sprintf("duplicate command '%s'", name))
			continue
		}
		seenCommands[name] = true

		cmdOK := true
		userControllable := false
		if ucVal, exists := cMap["userControllable"]; exists {
			if ucBool, ok := ucVal.(bool); ok {
				userControllable = ucBool
			} else {
				addErr(p+".userControllable", "must be a boolean")
				cmdOK = false
			}
		}

		params := make(map[string]TypeDef)
		if pVal, exists := cMap["params"]; exists {
			pMap, ok := pVal.(map[string]interface{})
			if !ok {
				addErr(p+".params", "must be an object")
				cmdOK = false
			} else {
				if len(pMap) > MaxParams {
					addErr(p+".params", fmt.Sprintf("at most %d params per command", MaxParams))
					cmdOK = false
				}
				for pn, pd := range pMap {
					pp := fmt.Sprintf("%s.params.%s", p, pn)
					if !NameRegex.MatchString(pn) {
						addErr(pp, "invalid param name")
						cmdOK = false
						continue
					}
					pdMap, ok := pd.(map[string]interface{})
					if !ok {
						addErr(pp, "must be an object")
						cmdOK = false
						continue
					}
					checkUnknownKeys(pdMap, []string{"type", "values", "min", "max"}, pp, addErr)
					tDef := parseTypeDef(pdMap, pp, addErr, false)
					if tDef == nil {
						cmdOK = false
					} else {
						params[pn] = *tDef
					}
				}
			}
		}

		effects := make(map[string]interface{})
		if eVal, exists := cMap["effects"]; exists {
			eMap, ok := eVal.(map[string]interface{})
			if !ok {
				addErr(p+".effects", "must be an object")
				cmdOK = false
			} else {
				for field, val := range eMap {
					ep := fmt.Sprintf("%s.effects.%s", p, field)
					sf, exists := stateByName[field]
					if !exists {
						addErr(ep, "unknown state field")
						cmdOK = false
						continue
					}
					sfType := TypeDef{Type: sf.Type, Values: sf.Values, Min: sf.Min, Max: sf.Max}

					if strVal, isStr := val.(string); isStr && strings.HasPrefix(strVal, "$") {
						pn := strVal[1:]
						pd, exists := params[pn]
						if !exists {
							addErr(ep, fmt.Sprintf("references unknown param '%s'", pn))
							cmdOK = false
						} else if pd.Type != sf.Type {
							addErr(ep, "param type differs from the state field type")
							cmdOK = false
						} else if sf.Type == "enum" {
							sfSet := make(map[string]bool)
							for _, v := range sf.Values {
								sfSet[v] = true
							}
							allIn := true
							for _, v := range pd.Values {
								if !sfSet[v] {
									allIn = false
									break
								}
							}
							if !allIn {
								addErr(ep, "param enum values must be a subset of the state field values")
								cmdOK = false
							} else {
								effects[field] = val
							}
						} else if sf.Type == "number" {
							if pd.Min == nil || pd.Max == nil || sf.Min == nil || sf.Max == nil || *pd.Min < *sf.Min || *pd.Max > *sf.Max {
								addErr(ep, "param range must lie within the state field range")
								cmdOK = false
							} else {
								effects[field] = val
							}
						} else {
							effects[field] = val
						}
					} else if !ValueFits(sfType, val) {
						addErr(ep, "constant does not fit the state field type/range")
						cmdOK = false
					} else {
						effects[field] = val
					}
				}
			}
		}

		if cmdOK {
			commands = append(commands, CommandSpec{
				Name:             name,
				Params:           params,
				Effects:          effects,
				UserControllable: userControllable,
			})
		}
	}

	if len(telemetry) == 0 && len(state) == 0 && len(telemetryIn) == 0 && len(stateIn) == 0 {
		addErr("", "at least one of telemetry or state must be non-empty")
	}

	if len(errors) > 0 {
		return ValidationResult{OK: false, Errors: errors, Spec: nil}
	}

	return ValidationResult{
		OK:     true,
		Errors: nil,
		Spec: &DeviceSpec{
			Type:        typeStr,
			Version:     version,
			Description: description,
			OfflineRate: offlineRate,
			Telemetry:   telemetry,
			State:       state,
			Commands:    commands,
		},
	}
}
