package bot

import (
	"fmt"

	"github.com/go-telegram/bot/models"
	"tbilisi-transport-tg-bot/internal/storage"
)

// reminderOptions are the selectable "remind me N minutes before arrival" values.
var reminderOptions = []int{5, 10, 15, 20, 25, 30}

// reminderKeyboard builds the inline keyboard for picking the reminder lead time.
func reminderKeyboard() *models.InlineKeyboardMarkup {
	var row []models.InlineKeyboardButton
	var rows [][]models.InlineKeyboardButton
	for i, m := range reminderOptions {
		row = append(row, models.InlineKeyboardButton{
			Text:         fmt.Sprintf("%d min", m),
			CallbackData: fmt.Sprintf("rem:%d", m),
		})
		if (i+1)%3 == 0 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// timeKeyboard builds a scrollable time picker. kind is "start" or "end".
// offset is the index of the first visible slot.
func timeKeyboard(kind string, offset int) *models.InlineKeyboardMarkup {
	if offset < 0 {
		offset = 0
	}
	if offset > totalSlots-windowSize {
		offset = totalSlots - windowSize
	}

	var times []models.InlineKeyboardButton
	for i := 0; i < windowSize; i++ {
		slot := offset + i
		minutes := slot * slotMinutes
		times = append(times, models.InlineKeyboardButton{
			Text:         formatMinutes(minutes),
			CallbackData: fmt.Sprintf("pick:%s:%d", kind, minutes),
		})
	}

	// Navigation row with arrows. At the bounds the arrow becomes an inert dot.
	left := models.InlineKeyboardButton{Text: "·", CallbackData: "noop"}
	if offset > 0 {
		prev := offset - windowSize
		if prev < 0 {
			prev = 0
		}
		left = models.InlineKeyboardButton{Text: "◀️", CallbackData: fmt.Sprintf("nav:%s:%d", kind, prev)}
	}

	right := models.InlineKeyboardButton{Text: "·", CallbackData: "noop"}
	if offset < totalSlots-windowSize {
		next := offset + windowSize
		if next > totalSlots-windowSize {
			next = totalSlots - windowSize
		}
		right = models.InlineKeyboardButton{Text: "▶️", CallbackData: fmt.Sprintf("nav:%s:%d", kind, next)}
	}

	nav := []models.InlineKeyboardButton{
		left,
		{Text: fmt.Sprintf("%s–%s", formatMinutes(offset*slotMinutes), formatMinutes((offset+windowSize-1)*slotMinutes)), CallbackData: "noop"},
		right,
	}

	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{times, nav}}
}

// stopsKeyboard builds a keyboard listing matched stops to choose from.
func stopsKeyboard(choices []storage.StopChoice) *models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton
	for i, c := range choices {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         stopLabel(c.Name, c.Code),
			CallbackData: fmt.Sprintf("stop:%d", i),
		}})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// removeKeyboard builds a keyboard listing the user's notifications for removal.
func removeKeyboard(ns []storage.Notification) *models.InlineKeyboardMarkup {
	var rows [][]models.InlineKeyboardButton
	for _, n := range ns {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         fmt.Sprintf("🗑 %s — bus %s @ %s", n.Name, n.BusNumber, stopLabel(n.StopName, n.StopCode)),
			CallbackData: fmt.Sprintf("del:%d", n.ID),
		}})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}
