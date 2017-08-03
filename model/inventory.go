package model

import (
	"log"
	"net/http"

	"github.com/codegangsta/martini-contrib/render"
	"github.com/go-martini/martini"
	"github.com/heartbeatsjp/happo-agent/lib"
	"github.com/heartbeatsjp/happo-agent/util"
)

// --- Constant Values

// --- Method

// Inventory execute command and collect inventory
func Inventory(inventoryRequest lib.InventoryRequest, r render.Render, params martini.Params) {
	var inventoryResponse lib.InventoryResponse

	if !util.Production {
		log.Printf("Inventory Command: %s %s\n", inventoryRequest.Command, inventoryRequest.CommandOption)
	}

	exitstatus, out, err := util.ExecCommandCombinedOutput(inventoryRequest.Command, inventoryRequest.CommandOption)
	if err != nil {
		r.JSON(http.StatusExpectationFailed, inventoryResponse)
		return
	}

	if exitstatus != 0 {
		inventoryResponse.ReturnCode = exitstatus
		inventoryResponse.ReturnValue = out
		r.JSON(http.StatusBadRequest, inventoryResponse)
		return
	}
	inventoryResponse.ReturnCode = exitstatus
	inventoryResponse.ReturnValue = out

	r.JSON(http.StatusOK, inventoryResponse)
}
