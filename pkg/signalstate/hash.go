package signalstate

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/sjzsdu/tongstock/pkg/ta"
	"hash"
	"math"
)

// DefinitionHash includes normalized parameters and algorithm/adjustment versions.
func DefinitionHash(def Definition) string {
	b, _ := json.Marshal(struct {
		Definition   Definition
		Calc, Adjust string
	}{def.Normalized(), CalcVersion, AdjustRaw})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func NewInputHasher() hash.Hash { return sha256.New() }

// HashInput uses the trading date (not timezone) and exact IEEE price/volume bits.
// Prefix hashing makes an upstream correction traceable even if the state is unchanged.
func HashInput(h hash.Hash, bar ta.KlineInput) {
	h.Write([]byte(bar.Time.Format("20060102")))
	var buf [8]byte
	for _, x := range []float64{bar.Open, bar.High, bar.Low, bar.Close, bar.Volume, bar.Amount} {
		binary.BigEndian.PutUint64(buf[:], math.Float64bits(x))
		h.Write(buf[:])
	}
}

func vectorHash(v StateVector, definition, prefix string) string {
	type dimension struct {
		Dim          Dim
		State        State
		Insufficient bool
	}
	dims := make([]dimension, 0, 4)
	for _, d := range Dims() {
		dims = append(dims, dimension{d, v.State(d), v.IsInsufficient(d)})
	}
	b, _ := json.Marshal(struct {
		Code, Date         string
		KType              uint8
		Definition, Prefix string
		Dims               []dimension
	}{v.Code, v.Date.Format("20060102"), v.KType, definition, prefix, dims})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
