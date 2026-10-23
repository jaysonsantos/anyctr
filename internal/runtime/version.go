package runtime

import "regexp"

var versionRe = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?(?:[-+][0-9A-Za-z.]+)?`)

// ExtractVersion pulls the first version-looking token out of a CLI's
// `--version` output. Every runtime prints something slightly different
// ("Docker version 27.3.1, build ce12230", "container CLI version 1.2.2 (...)"),
// and none of them are worth a bespoke parser.
func ExtractVersion(raw string) string {
	return versionRe.FindString(raw)
}
