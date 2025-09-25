package naming

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	mrand "math/rand"
)

var left = []string{
	"adventurous", "ancient", "ardent", "artful", "audacious", "august", "autumnal", "brave", "bright", "careful",
	"cheerful", "cunning", "dauntless", "devoted", "diligent", "doughty", "dour", "elven", "emerald", "enchanted",
	"enduring", "faithful", "farseeing", "fearless", "fiery", "gallant", "gentle", "gleaming", "golden", "goodhearted",
	"graceful", "grand", "greycloaked", "grim", "hale", "hardy", "heroic", "honest", "hopeful", "humble",
	"keen", "kind", "keeneyed", "laughing", "lightfoot", "loyal", "luminous", "merry", "mighty", "moonlit",
	"noble", "northern", "numenorean", "oaken", "patient", "proud", "quick", "quiet", "radiant", "resolute",
	"resourceful", "righteous", "rugged", "sage", "scarlet", "shadowwise", "silent", "silver", "simple", "steadfast",
	"stern", "stout", "strong", "sturdy", "subtle", "surefoot", "swift", "tall", "thoughtful", "thunderous",
	"tireless", "true", "twilight", "valiant", "vigilant", "wandering", "warmhearted", "wary", "weathered", "white",
	"wild", "willowy", "wise", "wry", "youthful", "starcrowned", "seafaring", "westering", "windborne", "woodwise",
}

var right = []string{
	"frodo", "samwise", "aragorn", "legolas", "gimli", "gandalf", "boromir", "faramir", "eowyn", "eomer",
	"theoden", "denethor", "galadriel", "elrond", "arwen", "celeborn", "haldir", "tauriel", "thranduil", "bilbo",
	"balin", "dwalin", "fili", "kili", "thorin", "bard", "beorn", "smaug", "gollum", "smeagol",
	"saruman", "grima", "treebeard", "quickbeam", "tombombadil", "goldberry", "radagast", "melian", "thingol", "beren",
	"luthien", "earendil", "elros", "elendil", "isildur", "anarion", "gilgalad", "cirdan", "glorfindel", "miriel",
	"finwe", "feanor", "fingolfin", "finarfin", "turgon", "gondolin", "tuor", "idril", "maeglin", "eol",
	"morgoth", "sauron", "shelob", "ungoliant", "gothmog", "balrog", "melkor", "numenor", "valinor", "anduril",
	"narsil", "sting", "glamdring", "orcrist", "rivendell", "lorien", "lothlorien", "mirkwood", "fangorn", "rohan",
	"gondor", "osgiliath", "minastirith", "minasmorgul", "angmar", "erebor", "edoras", "helmsdeep", "isengard", "moria",
	"khazaddum", "caradhras", "amonhen", "amonsul", "fornost", "weathertop", "bucklebury", "hobbiton", "bywater", "bagend",
}

func RandomSessionName() string {
	r := mrand.New(mrand.NewSource(randSeed()))
	l := left[r.Intn(len(left))]
	rn := right[r.Intn(len(right))]
	n := l + "_" + rn
	return n
}

func randSeed() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return int64(binary.LittleEndian.Uint64(b[:]))
	}
	return mrand.Int63()
}
func RandomID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
