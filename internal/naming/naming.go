// Copyright 2025 Emiliano Spinella (eminwux)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package naming

import (
	"crypto/rand"
	"encoding/hex"
	mrand "math/rand/v2"
)

//nolint:gochecknoglobals // word lists
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

//nolint:gochecknoglobals // word lists
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

// RandomName returns a human-friendly "<adjective>_<name>" terminal name.
// math/rand/v2's top-level functions are randomly seeded per process, so no
// explicit seeding is needed.
func RandomName() string {
	//nolint:gosec // cosmetic display name, not a security token; RandomID covers uniqueness
	l := left[mrand.IntN(len(left))]
	//nolint:gosec // cosmetic display name, not a security token; RandomID covers uniqueness
	rn := right[mrand.IntN(len(right))]
	return l + "_" + rn
}

func RandomID() string {
	length := 4
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
