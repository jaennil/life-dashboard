package connectors

import (
	"encoding/json"
	"testing"
)

// Every payload below is a verbatim capture from the live account, trimmed only
// of the long sample arrays. The API sends every value as a string, which is the
// whole reason these sections used to decode into nothing.
const (
	liveStressItem = `{"userId":"8725027931","eventType":"all_day_stress","subType":"all_day_stress",
		"timestamp":1789160400001,"minStress":"1","deviceType":"0","mediumProportion":"2","maxStress":"63",
		"data":"[]","relaxProportion":"95","deviceId":"C369A9FFFEE85BA2","avgStress":"12","highProportion":"0",
		"normalProportion":"3","deviceMac":"C369A9E85BA2"}`

	liveOxygenODI = `{"userId":"8725027931","eventType":"blood_oxygen","subType":"odi","timestamp":1789160400000,
		"valid":"2","score":"99","cost":"29940","dispCode":"1","odi":"0.47999999999999998","timezone":"Europe/Moscow",
		"deviceId":"C369A9FFFEE85BA2","odiNum":"4"}`

	livePAIItem = `{"userId":"8725027931","eventType":"PaiHealthInfo","subType":"PaiHealthInfo","timestamp":1789160400000,
		"mediumZoneLowerLimit":"118","mediumZonePai":"2.3636856079101562","gender":"0","lowZoneLowerLimit":"98",
		"mediumZoneMinutes":"22","maxHr":"197","totalPai":"26.818082809448242","highZonePai":"0","index":"2",
		"timeZone":"12","version":"5","restHr":"56","lowZonePai":"3"}`
)

func TestZeppStressItemDecodesQuotedNumbers(t *testing.T) {
	var item zeppStressItem
	if err := json.Unmarshal([]byte(liveStressItem), &item); err != nil {
		t.Fatalf("live stress payload did not decode: %v", err)
	}

	if item.Avg.float() != 12 || item.Min.float() != 1 || item.Max.float() != 63 {
		t.Errorf("stress values = avg %v, min %v, max %v", item.Avg, item.Min, item.Max)
	}
	if item.Relax.float() != 95 || item.Normal.float() != 3 || item.Medium.float() != 2 || item.High.float() != 0 {
		t.Errorf("zone shares = %v %v %v %v", item.Relax, item.Normal, item.Medium, item.High)
	}
	if item.Timestamp.int64() != 1789160400001 {
		t.Errorf("timestamp = %d", item.Timestamp.int64())
	}
}

func TestZeppPAIItemDecodesQuotedNumbers(t *testing.T) {
	var item zeppPAIItem
	if err := json.Unmarshal([]byte(livePAIItem), &item); err != nil {
		t.Fatalf("live PAI payload did not decode: %v", err)
	}
	if got := item.TotalPAI.float(); got < 26.8 || got > 26.9 {
		t.Errorf("total pai = %v", got)
	}
	if item.RestHR.float() != 56 || item.MaxHR.float() != 197 {
		t.Errorf("heart rates = rest %v, max %v", item.RestHR, item.MaxHR)
	}
	if item.LowZone.float() != 3 || item.HighZone.float() != 0 {
		t.Errorf("zone pai = low %v, high %v", item.LowZone, item.HighZone)
	}
}

func TestZeppOxygenSeparatesTheIndexFromASaturation(t *testing.T) {
	var item zeppOxygenItem
	if err := json.Unmarshal([]byte(liveOxygenODI), &item); err != nil {
		t.Fatalf("live blood oxygen payload did not decode: %v", err)
	}

	// The overnight item carries no saturation percentage at all. The "score" of
	// 99 in it is not one, and filing it as spo2 would invent a measurement.
	if _, ok := zeppOxygenValue(item); ok {
		t.Error("an overnight index was read as a saturation reading")
	}
	if item.SubType != "odi" || item.ODINum.float() != 4 {
		t.Errorf("item = %+v", item)
	}
	if got := item.ODI.float(); got < 0.47 || got > 0.49 {
		t.Errorf("odi = %v", got)
	}
}

func TestZeppOxygenStillReadsASpotReading(t *testing.T) {
	// The shape the band produced when measurements were taken by hand, which is
	// where every stored spo2 row came from.
	var item zeppOxygenItem
	spot := `{"subType":"click","timestamp":1707225600000,"extra":"{\"spo2\":97}"}`
	if err := json.Unmarshal([]byte(spot), &item); err != nil {
		t.Fatal(err)
	}
	value, ok := zeppOxygenValue(item)
	if !ok || value != 97 {
		t.Fatalf("spot reading = %v, %v", value, ok)
	}
}

func TestZeppNumberHandlesTheEmptyCases(t *testing.T) {
	var payload struct {
		A zeppNumber `json:"a"`
		B zeppNumber `json:"b"`
		C zeppNumber `json:"c"`
		D zeppNumber `json:"d"`
	}
	if err := json.Unmarshal([]byte(`{"a":null,"b":"","c":7,"d":"7.5"}`), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.A != 0 || payload.B != 0 || payload.C != 7 || payload.D != 7.5 {
		t.Fatalf("values = %v %v %v %v", payload.A, payload.B, payload.C, payload.D)
	}

	// Garbage must fail loudly rather than silently become zero.
	var broken struct {
		A zeppNumber `json:"a"`
	}
	if err := json.Unmarshal([]byte(`{"a":"не число"}`), &broken); err == nil {
		t.Fatal("an unparsable number was accepted")
	}
}
