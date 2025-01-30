package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/migrations"
)

const webhooksCollection = "_webhooks"

type Webhook struct {
	ID          string `db:"id" json:"id"`
	Name        string `db:"name" json:"name"`
	Collection  string `db:"collection" json:"collection"`
	Destination string `db:"destination" json:"destination"`
}

func attachWebhooks(app *pocketbase.PocketBase) {
	migrations.Register(func(app core.App) error {
		collection := core.NewBaseCollection(webhooksCollection)
		collection.System = true

		collection.Fields.Add(&core.TextField{
			Name:     "name",
			Required: true,
		})

		collection.Fields.Add(&core.TextField{
			Name:     "collection",
			Required: true,
		})

		collection.Fields.Add(&core.URLField{
			Name:     "destination",
			Required: true,
		})

		return app.Save(collection)

	}, func(app core.App) error {

		id, err := app.FindCollectionByNameOrId(webhooksCollection)
		if err != nil {
			return err
		}

		return app.Delete(id)
	}, "1690000000_webhooks.go")

	// app.OnRecordCreateRequest().BindFunc(func(e *core.RecordRequestEvent) error {
	// 	error := e.Next()

	// 	if error != nil {
	// 		log.Println(error)
	// 		return nil
	// 	}

	// 	return event(app, "create", e.Collection.Name, e.Record)
	// })
	// app.OnRecordUpdateRequest().BindFunc(func(e *core.RecordRequestEvent) error {
	// 	event(app, "update", e.Collection.Name, e.Record)
	// 	return e.Next()
	// })
	// app.OnRecordDeleteRequest().BindFunc(func(e *core.RecordRequestEvent) error {
	// 	event(app, "delete", e.Collection.Name, e.Record)
	// 	return e.Next()
	// })

	app.OnRecordAfterCreateSuccess().BindFunc(func(e *core.RecordEvent) error {

		event(app, "create-after-success", e.Record.Collection().Name, e.Record)

		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess().BindFunc(func(e *core.RecordEvent) error {

		event(app, "update-after-success", e.Record.Collection().Name, e.Record)

		return e.Next()
	})

	app.OnRecordAfterDeleteSuccess().BindFunc(func(e *core.RecordEvent) error {

		event(app, "delete-after-success", e.Record.Collection().Name, e.Record)

		return e.Next()
	})
}

type Payload struct {
	Action     string       `json:"action"`
	Collection string       `json:"collection"`
	Record     *core.Record `json:"record"`
	Auth       *core.Record `json:"auth,omitempty"`
}

func event(app *pocketbase.PocketBase, action, collection string, record *core.Record) error {

	var webhooks []Webhook
	if err := app.DB().
		Select().
		From(webhooksCollection).
		Where(dbx.HashExp{"collection": collection}).
		All(&webhooks); err != nil {
		return err
	}

	if len(webhooks) == 0 {
		return nil
	}

	payload, err := json.Marshal(&Payload{
		Action:     action,
		Collection: collection,
		Record:     record,
	})
	if err != nil {
		return err
	}

	for _, webhook := range webhooks {
		if err := sendWebhook(webhook, payload); err != nil {
			app.Logger().Error("failed to send webhook", "action", action, "name", webhook.Name, "collection", webhook.Collection, "destination", webhook.Destination, "error", err.Error())
		} else {
			app.Logger().Info("webhook sent", "action", action, "name", webhook.Name, "collection", webhook.Collection, "destination", webhook.Destination)
		}
	}

	return nil
}

func sendWebhook(webhook Webhook, payload []byte) error {
	req, _ := http.NewRequest(http.MethodPost, webhook.Destination, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)

		return fmt.Errorf("failed to send webhook: %s", string(b))
	}

	return nil
}
