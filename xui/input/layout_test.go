package input

import "testing"

// TestLayoutLatinMapsRussianKeys pins the reverse ЙЦУКЕН map: the letter a
// Russian layout produces must map back to the Latin letter the same physical
// key produces on US-QWERTY (case preserved).
func TestLayoutLatinMapsRussianKeys(t *testing.T) {
	cases := map[rune]rune{
		'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y', 'г': 'u',
		'ш': 'i', 'щ': 'o', 'з': 'p', 'х': '[', 'ъ': ']',
		'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h', 'о': 'j',
		'л': 'k', 'д': 'l', 'ж': ';', 'э': '\'',
		'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n', 'ь': 'm',
		'б': ',', 'ю': '.', 'ё': '`',
		'Й': 'Q', 'Ц': 'W', 'У': 'E', 'К': 'R', 'Е': 'T', 'Н': 'Y', 'Г': 'U',
		'Ш': 'I', 'Щ': 'O', 'З': 'P', 'Х': '{', 'Ъ': '}',
		'Ф': 'A', 'Ы': 'S', 'В': 'D', 'А': 'F', 'П': 'G', 'Р': 'H', 'О': 'J',
		'Л': 'K', 'Д': 'L', 'Ж': ':', 'Э': '"',
		'Я': 'Z', 'Ч': 'X', 'С': 'C', 'М': 'V', 'И': 'B', 'Т': 'N', 'Ь': 'M',
		'Б': '<', 'Ю': '>', 'Ё': '~',
	}
	for cyr, want := range cases {
		if got := layoutLatin(cyr); got != want {
			t.Errorf("layoutLatin(%q) = %q, want %q", cyr, got, want)
		}
	}
}

// TestLayoutLatinPassesThroughLatin keeps Latin and non-letters untouched so
// text entry and already-correct terminals never change behaviour.
func TestLayoutLatinPassesThroughLatin(t *testing.T) {
	for _, r := range []rune{'a', 'Z', 'k', 'K', 'q', '1', ' ', 0} {
		if got := layoutLatin(r); got != r {
			t.Errorf("layoutLatin(%q) = %q, want unchanged", r, got)
		}
	}
}
