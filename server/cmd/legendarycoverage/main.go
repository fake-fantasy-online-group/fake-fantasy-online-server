// Command legendarycoverage reports typed runtime registration separately from
// reviewed drop eligibility. It does not execute prose or certify handlers.
package main

import (
	"encoding/json"
	"github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/game/scene"
	"os"
)

func main() {
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	if err := e.Encode(scene.LegendaryRuntimeCoverage()); err != nil {
		panic(err)
	}
}
