package profiler

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/google/pprof/profile"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
)

// toOTLP converts a parsed pprof profile into the OTLP profiles signal. An
// OTLP profile holds exactly one sample type, so a pprof profile with N
// sample types (heap has four) becomes N OTLP profile records sharing one
// dictionary. Samples labelled with trace_id/span_id (set by TracedProfiler)
// are linked to their span through the link table.
//
// The original pprof payload is intentionally not attached: it would bypass
// sanitization.
func toOTLP(src *profile.Profile, pt ProfileType, res *resource.Resource) (pprofile.Profiles, error) {
	out := pprofile.NewProfiles()

	c := converter{dict: out.Dictionary(), strings: make(map[string]int32)}
	if err := c.seedDictionary(); err != nil {
		return out, err
	}

	rp := out.ResourceProfiles().AppendEmpty()
	copyResource(res, rp.Resource())
	rp.SetSchemaUrl(res.SchemaURL())

	sp := rp.ScopeProfiles().AppendEmpty()
	sp.Scope().SetName(scopeName)

	// per-sample data shared by all sample types
	stacks := make([]int32, len(src.Sample))
	attrs := make([][]int32, len(src.Sample))
	links := make([]int32, len(src.Sample))
	for i, s := range src.Sample {
		var err error
		if stacks[i], err = c.stack(s); err != nil {
			return out, err
		}
		if attrs[i], err = c.sampleAttributes(s); err != nil {
			return out, err
		}
		if links[i], err = c.link(s); err != nil {
			return out, err
		}
	}

	profileTypeAttr, err := c.stringAttribute(profileTypeKey, string(pt))
	if err != nil {
		return out, err
	}

	// Go emits the identical pprof schema for the block and mutex profiles
	// (contentions/count + delay/nanoseconds with period type "contentions"),
	// and backends derive the profile's display name from the period type —
	// the two would merge into one indistinguishable type. Naming the period
	// type after the profile keeps them apart.
	periodType := src.PeriodType
	if periodType != nil && (pt == ProfileTypeMutex || pt == ProfileTypeBlock) {
		periodType = &profile.ValueType{Type: string(pt), Unit: periodType.Unit}
	}

	for typeIndex, sampleType := range src.SampleType {
		p := sp.Profiles().AppendEmpty()

		id, err := newProfileID()
		if err != nil {
			return out, err
		}
		p.SetProfileID(id)

		if err := c.valueType(p.SampleType(), sampleType); err != nil {
			return out, err
		}
		if periodType != nil {
			if err := c.valueType(p.PeriodType(), periodType); err != nil {
				return out, err
			}
			p.SetPeriod(src.Period)
		}

		p.SetTime(pcommon.Timestamp(src.TimeNanos))
		if src.DurationNanos > 0 {
			p.SetDurationNano(uint64(src.DurationNanos))
		}
		p.AttributeIndices().Append(profileTypeAttr)

		for i, s := range src.Sample {
			sample := p.Samples().AppendEmpty()
			sample.SetStackIndex(stacks[i])
			sample.Values().Append(s.Value[typeIndex])
			sample.AttributeIndices().Append(attrs[i]...)
			if links[i] != 0 {
				sample.SetLinkIndex(links[i])
			}
		}
	}

	return out, nil
}

// converter builds up the profiles dictionary. Mappings, functions and
// locations are appended directly (pprof already deduplicates them by ID);
// stacks, links and attributes go through the Set helpers which deduplicate
// by value.
type converter struct {
	dict    pprofile.ProfilesDictionary
	strings map[string]int32

	mappings  map[uint64]int32
	functions map[uint64]int32
	locations map[uint64]int32
}

// seedDictionary creates the zero entry every dictionary table must hold at
// index 0, so unset references resolve to an empty value.
func (c *converter) seedDictionary() error {
	c.mappings = make(map[uint64]int32)
	c.functions = make(map[uint64]int32)
	c.locations = make(map[uint64]int32)

	if _, err := pprofile.SetString(c.dict.StringTable(), ""); err != nil {
		return err
	}
	c.strings[""] = 0

	if _, err := pprofile.SetMapping(c.dict.MappingTable(), pprofile.NewMapping()); err != nil {
		return err
	}
	if _, err := pprofile.SetFunction(c.dict.FunctionTable(), pprofile.NewFunction()); err != nil {
		return err
	}
	if _, err := pprofile.SetLocation(c.dict.LocationTable(), pprofile.NewLocation()); err != nil {
		return err
	}
	if _, err := pprofile.SetStack(c.dict.StackTable(), pprofile.NewStack()); err != nil {
		return err
	}
	if _, err := pprofile.SetLink(c.dict.LinkTable(), pprofile.NewLink()); err != nil {
		return err
	}
	if _, err := pprofile.SetAttribute(c.dict.AttributeTable(), pprofile.NewKeyValueAndUnit()); err != nil {
		return err
	}

	return nil
}

func (c *converter) string(v string) (int32, error) {
	if idx, ok := c.strings[v]; ok {
		return idx, nil
	}

	idx, err := pprofile.SetString(c.dict.StringTable(), v)
	if err != nil {
		return 0, err
	}
	c.strings[v] = idx

	return idx, nil
}

func (c *converter) valueType(dst pprofile.ValueType, src *profile.ValueType) error {
	typeIdx, err := c.string(src.Type)
	if err != nil {
		return err
	}
	unitIdx, err := c.string(src.Unit)
	if err != nil {
		return err
	}

	dst.SetTypeStrindex(typeIdx)
	dst.SetUnitStrindex(unitIdx)

	return nil
}

func (c *converter) mapping(m *profile.Mapping) (int32, error) {
	if idx, ok := c.mappings[m.ID]; ok {
		return idx, nil
	}

	fileIdx, err := c.string(m.File)
	if err != nil {
		return 0, err
	}

	om := pprofile.NewMapping()
	om.SetMemoryStart(m.Start)
	om.SetMemoryLimit(m.Limit)
	om.SetFileOffset(m.Offset)
	om.SetFilenameStrindex(fileIdx)

	idx := int32(c.dict.MappingTable().Len())
	om.MoveTo(c.dict.MappingTable().AppendEmpty())
	c.mappings[m.ID] = idx

	return idx, nil
}

func (c *converter) function(f *profile.Function) (int32, error) {
	if idx, ok := c.functions[f.ID]; ok {
		return idx, nil
	}

	nameIdx, err := c.string(f.Name)
	if err != nil {
		return 0, err
	}
	systemNameIdx, err := c.string(f.SystemName)
	if err != nil {
		return 0, err
	}
	fileIdx, err := c.string(f.Filename)
	if err != nil {
		return 0, err
	}

	of := pprofile.NewFunction()
	of.SetNameStrindex(nameIdx)
	of.SetSystemNameStrindex(systemNameIdx)
	of.SetFilenameStrindex(fileIdx)
	of.SetStartLine(f.StartLine)

	idx := int32(c.dict.FunctionTable().Len())
	of.MoveTo(c.dict.FunctionTable().AppendEmpty())
	c.functions[f.ID] = idx

	return idx, nil
}

func (c *converter) location(l *profile.Location) (int32, error) {
	if idx, ok := c.locations[l.ID]; ok {
		return idx, nil
	}

	ol := pprofile.NewLocation()
	ol.SetAddress(l.Address)
	if l.Mapping != nil {
		mappingIdx, err := c.mapping(l.Mapping)
		if err != nil {
			return 0, err
		}
		ol.SetMappingIndex(mappingIdx)
	}

	for _, line := range l.Line {
		oline := ol.Lines().AppendEmpty()
		oline.SetLine(line.Line)
		oline.SetColumn(line.Column)
		if line.Function != nil {
			functionIdx, err := c.function(line.Function)
			if err != nil {
				return 0, err
			}
			oline.SetFunctionIndex(functionIdx)
		}
	}

	idx := int32(c.dict.LocationTable().Len())
	ol.MoveTo(c.dict.LocationTable().AppendEmpty())
	c.locations[l.ID] = idx

	return idx, nil
}

// stack converts a sample's call stack; both pprof and OTLP order locations
// leaf first.
func (c *converter) stack(s *profile.Sample) (int32, error) {
	stack := pprofile.NewStack()
	stack.LocationIndices().EnsureCapacity(len(s.Location))
	for _, loc := range s.Location {
		idx, err := c.location(loc)
		if err != nil {
			return 0, err
		}
		stack.LocationIndices().Append(idx)
	}

	return pprofile.SetStack(c.dict.StackTable(), stack)
}

// sampleAttributes converts the sample's pprof labels to attribute-table
// references. The trace correlation labels are skipped: they become links.
func (c *converter) sampleAttributes(s *profile.Sample) ([]int32, error) {
	var indices []int32

	for key, values := range s.Label {
		if key == traceIDLabelKey || key == spanIDLabelKey {
			continue
		}

		for _, v := range values {
			idx, err := c.stringAttribute(key, v)
			if err != nil {
				return nil, err
			}
			indices = append(indices, idx)
		}
	}

	for key, values := range s.NumLabel {
		keyIdx, err := c.string(key)
		if err != nil {
			return nil, err
		}

		units := s.NumUnit[key]
		for i, v := range values {
			kv := pprofile.NewKeyValueAndUnit()
			kv.SetKeyStrindex(keyIdx)
			kv.Value().SetInt(v)
			if i < len(units) && units[i] != "" {
				unitIdx, err := c.string(units[i])
				if err != nil {
					return nil, err
				}
				kv.SetUnitStrindex(unitIdx)
			}

			idx, err := pprofile.SetAttribute(c.dict.AttributeTable(), kv)
			if err != nil {
				return nil, err
			}
			indices = append(indices, idx)
		}
	}

	return indices, nil
}

func (c *converter) stringAttribute(key, value string) (int32, error) {
	keyIdx, err := c.string(key)
	if err != nil {
		return 0, err
	}

	kv := pprofile.NewKeyValueAndUnit()
	kv.SetKeyStrindex(keyIdx)
	kv.Value().SetStr(value)

	return pprofile.SetAttribute(c.dict.AttributeTable(), kv)
}

// link converts the trace_id/span_id pprof labels into a link-table entry; 0
// (the zero link) means the sample is not correlated with a span.
func (c *converter) link(s *profile.Sample) (int32, error) {
	traceIDs := s.Label[traceIDLabelKey]
	spanIDs := s.Label[spanIDLabelKey]
	if len(traceIDs) == 0 || len(spanIDs) == 0 {
		return 0, nil
	}

	var traceID pcommon.TraceID
	if raw, err := hex.DecodeString(traceIDs[0]); err != nil || len(raw) != len(traceID) {
		return 0, nil
	} else {
		copy(traceID[:], raw)
	}

	var spanID pcommon.SpanID
	if raw, err := hex.DecodeString(spanIDs[0]); err != nil || len(raw) != len(spanID) {
		return 0, nil
	} else {
		copy(spanID[:], raw)
	}

	link := pprofile.NewLink()
	link.SetTraceID(traceID)
	link.SetSpanID(spanID)

	return pprofile.SetLink(c.dict.LinkTable(), link)
}

// copyResource mirrors the SDK resource (service.name, host, process, ...)
// onto the OTLP resource so profiles join the service's other signals.
func copyResource(res *resource.Resource, dst pcommon.Resource) {
	attrs := dst.Attributes()
	for _, kv := range res.Attributes() {
		key := string(kv.Key)
		switch kv.Value.Type() {
		case attribute.STRING:
			attrs.PutStr(key, kv.Value.AsString())
		case attribute.BOOL:
			attrs.PutBool(key, kv.Value.AsBool())
		case attribute.INT64:
			attrs.PutInt(key, kv.Value.AsInt64())
		case attribute.FLOAT64:
			attrs.PutDouble(key, kv.Value.AsFloat64())
		default:
			attrs.PutStr(key, kv.Value.Emit())
		}
	}
}

func newProfileID() (pprofile.ProfileID, error) {
	var id pprofile.ProfileID
	if _, err := rand.Read(id[:]); err != nil {
		return id, fmt.Errorf("profiler: generating profile id: %w", err)
	}

	return id, nil
}
