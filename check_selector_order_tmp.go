package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"deck/internal/settings"
)

func main() {
	root, err := os.MkdirTemp("", "deck-selector-order-")
	if err != nil { panic(err) }
	file := filepath.Join(root, "setting.json")
	write := func(raw string) { if err := os.WriteFile(file, []byte(raw), 0600); err != nil { panic(err) } }
	check := func(store *settings.Store, want []string) {
		got := store.Prefs().Applications[settings.Codex].ModelSelectorOrder
		if !reflect.DeepEqual(got, want) { panic(fmt.Sprintf("order: %v != %v", got, want)) }
	}
	write(`{"applications":{"codex":{"modelSelector":{"Z 中":{"model":"z","reasoningEffort":"medium"},"10":{"model":"ten","reasoningEffort":"high"},"2":{"model":"two","reasoningEffort":"low"},"A 中":{"model":"a","reasoningEffort":"medium"}}}}}`)
	store := settings.NewStore(root)
	store.Load()
	check(store, []string{"Z 中", "10", "2", "A 中"})
	reloaded := settings.NewStore(root)
	reloaded.Load()
	check(reloaded, []string{"Z 中", "10", "2", "A 中"})
	write(`{"applications":{"codex":{"modelSelector":{"2":{"model":"two","reasoningEffort":"low"},"Z 中":{"model":"z","reasoningEffort":"medium"}}}}}`)
	if err := <-store.Update(func(p settings.Preferences) settings.Preferences { p.Theme = "dark"; return p }); err != nil { panic(err) }
	check(store, []string{"2", "Z 中"})
	reloaded.Load()
	check(reloaded, []string{"2", "Z 中"})
	fmt.Println("配置书写顺序、数字名称、保存前重读、写回后重启顺序：通过")
}
