package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/VolkovDani/project-creator/internal/processgraph"
)

type processGraphProgressEvent struct {
	SchemaVersion int                     `json:"schemaVersion"`
	ProjectID     string                  `json:"projectId"`
	Status        string                  `json:"status"`
	Run           processGraphProgressRun `json:"run"`
}

type processGraphProgressRun struct {
	ID        string `json:"id"`
	Topic     string `json:"topic"`
	Revision  int    `json:"revision"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Failed    int    `json:"failed"`
}

func writeProcessGraphProgress(w http.ResponseWriter, snapshot processgraph.BackgroundProgress) error {
	payload, err := json.Marshal(processGraphProgressEvent{
		SchemaVersion: 1,
		ProjectID:     snapshot.ProjectID,
		Status:        snapshot.Status,
		Run: processGraphProgressRun{
			ID:        snapshot.Run.ID,
			Topic:     snapshot.Run.Topic,
			Revision:  snapshot.Run.Revision,
			Completed: snapshot.Run.Completed,
			Total:     snapshot.Run.Total,
			Failed:    snapshot.Run.Failed,
		},
	})
	if err != nil {
		return err
	}
	// The snapshot is ephemeral and does not advance the persisted SSE cursor.
	_, err = fmt.Fprintf(w, "event: process_graph.progress\ndata: %s\n\n", payload)
	return err
}
