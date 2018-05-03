package transfers

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/vechain/thor/api/utils"
	"github.com/vechain/thor/logdb"
)

type Transfers struct {
	db *logdb.LogDB
}

func New(db *logdb.LogDB) *Transfers {
	return &Transfers{
		db,
	}
}

//Filter query logs with option
func (t *Transfers) filter(ctx context.Context, filter *logdb.TransferFilter) ([]*FilteredTransfer, error) {
	transfers, err := t.db.FilterTransfers(ctx, filter)
	if err != nil {
		return nil, err
	}
	tLogs := make([]*FilteredTransfer, len(transfers))
	for i, trans := range transfers {
		tLogs[i] = ConvertTransfer(trans)
	}
	return tLogs, nil
}

func (t *Transfers) handleFilterTransferLogs(w http.ResponseWriter, req *http.Request) error {
	res, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return err
	}
	req.Body.Close()
	var filter logdb.TransferFilter
	if err := json.Unmarshal(res, &filter); err != nil {
		return err
	}
	order := req.URL.Query().Get("order")
	if order != string(logdb.DESC) {
		filter.Order = logdb.ASC
	} else {
		filter.Order = logdb.DESC
	}
	tLogs, err := t.filter(req.Context(), &filter)
	if err != nil {
		return err
	}
	return utils.WriteJSON(w, tLogs)
}

func (t *Transfers) Mount(root *mux.Router, pathPrefix string) {
	sub := root.PathPrefix(pathPrefix).Subrouter()

	sub.Path("").Methods("POST").HandlerFunc(utils.WrapHandlerFunc(t.handleFilterTransferLogs))
}