// Copyright 2024, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wstore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/google/uuid"
	"github.com/wavetermdev/thenextwave/pkg/shellexec"
	"github.com/wavetermdev/thenextwave/pkg/waveobj"
)

var waveObjUpdateKey = struct{}{}

func init() {
	for _, rtype := range AllWaveObjTypes() {
		waveobj.RegisterType(rtype)
	}
}

type contextUpdatesType struct {
	UpdatesStack []map[waveobj.ORef]WaveObjUpdate
}

func dumpUpdateStack(updates *contextUpdatesType) {
	log.Printf("dumpUpdateStack len:%d\n", len(updates.UpdatesStack))
	for idx, update := range updates.UpdatesStack {
		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("  [%d]:", idx))
		for k := range update {
			buf.WriteString(fmt.Sprintf(" %s:%s", k.OType, k.OID))
		}
		buf.WriteString("\n")
		log.Print(buf.String())
	}
}

func ContextWithUpdates(ctx context.Context) context.Context {
	updatesVal := ctx.Value(waveObjUpdateKey)
	if updatesVal != nil {
		return ctx
	}
	return context.WithValue(ctx, waveObjUpdateKey, &contextUpdatesType{
		UpdatesStack: []map[waveobj.ORef]WaveObjUpdate{make(map[waveobj.ORef]WaveObjUpdate)},
	})
}

func ContextGetUpdates(ctx context.Context) map[waveobj.ORef]WaveObjUpdate {
	updatesVal := ctx.Value(waveObjUpdateKey)
	if updatesVal == nil {
		return nil
	}
	updates := updatesVal.(*contextUpdatesType)
	if len(updates.UpdatesStack) == 1 {
		return updates.UpdatesStack[0]
	}
	rtn := make(map[waveobj.ORef]WaveObjUpdate)
	for _, update := range updates.UpdatesStack {
		for k, v := range update {
			rtn[k] = v
		}
	}
	return rtn
}

func ContextGetUpdate(ctx context.Context, oref waveobj.ORef) *WaveObjUpdate {
	updatesVal := ctx.Value(waveObjUpdateKey)
	if updatesVal == nil {
		return nil
	}
	updates := updatesVal.(*contextUpdatesType)
	for idx := len(updates.UpdatesStack) - 1; idx >= 0; idx-- {
		if obj, ok := updates.UpdatesStack[idx][oref]; ok {
			return &obj
		}
	}
	return nil
}

func ContextAddUpdate(ctx context.Context, update WaveObjUpdate) {
	updatesVal := ctx.Value(waveObjUpdateKey)
	if updatesVal == nil {
		return
	}
	updates := updatesVal.(*contextUpdatesType)
	oref := waveobj.ORef{
		OType: update.OType,
		OID:   update.OID,
	}
	updates.UpdatesStack[len(updates.UpdatesStack)-1][oref] = update
}

func ContextUpdatesBeginTx(ctx context.Context) context.Context {
	updatesVal := ctx.Value(waveObjUpdateKey)
	if updatesVal == nil {
		return ctx
	}
	updates := updatesVal.(*contextUpdatesType)
	updates.UpdatesStack = append(updates.UpdatesStack, make(map[waveobj.ORef]WaveObjUpdate))
	return ctx
}

func ContextUpdatesCommitTx(ctx context.Context) {
	updatesVal := ctx.Value(waveObjUpdateKey)
	if updatesVal == nil {
		return
	}
	updates := updatesVal.(*contextUpdatesType)
	if len(updates.UpdatesStack) <= 1 {
		panic(fmt.Errorf("no updates transaction to commit"))
	}
	// merge the last two updates
	curUpdateMap := updates.UpdatesStack[len(updates.UpdatesStack)-1]
	prevUpdateMap := updates.UpdatesStack[len(updates.UpdatesStack)-2]
	for k, v := range curUpdateMap {
		prevUpdateMap[k] = v
	}
	updates.UpdatesStack = updates.UpdatesStack[:len(updates.UpdatesStack)-1]
}

func ContextUpdatesRollbackTx(ctx context.Context) {
	updatesVal := ctx.Value(waveObjUpdateKey)
	if updatesVal == nil {
		return
	}
	updates := updatesVal.(*contextUpdatesType)
	if len(updates.UpdatesStack) <= 1 {
		panic(fmt.Errorf("no updates transaction to rollback"))
	}
	updates.UpdatesStack = updates.UpdatesStack[:len(updates.UpdatesStack)-1]
}

type WaveObjTombstone struct {
	OType string `json:"otype"`
	OID   string `json:"oid"`
}

const (
	UpdateType_Update = "update"
	UpdateType_Delete = "delete"
)

type WaveObjUpdate struct {
	UpdateType string          `json:"updatetype"`
	OType      string          `json:"otype"`
	OID        string          `json:"oid"`
	Obj        waveobj.WaveObj `json:"obj,omitempty"`
}

func (update WaveObjUpdate) MarshalJSON() ([]byte, error) {
	rtn := make(map[string]any)
	rtn["updatetype"] = update.UpdateType
	rtn["otype"] = update.OType
	rtn["oid"] = update.OID
	var err error
	rtn["obj"], err = waveobj.ToJsonMap(update.Obj)
	if err != nil {
		return nil, err
	}
	return json.Marshal(rtn)
}

type UIContext struct {
	WindowId    string `json:"windowid"`
	ActiveTabId string `json:"activetabid"`
}

type Client struct {
	OID          string `json:"oid"`
	Version      int    `json:"version"`
	MainWindowId string `json:"mainwindowid"`
}

func (*Client) GetOType() string {
	return "client"
}

func AllWaveObjTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf(&Client{}),
		reflect.TypeOf(&Window{}),
		reflect.TypeOf(&Workspace{}),
		reflect.TypeOf(&Tab{}),
		reflect.TypeOf(&Block{}),
	}
}

// stores the ui-context of the window
// workspaceid, active tab, active block within each tab, window size, etc.
type Window struct {
	OID            string            `json:"oid"`
	Version        int               `json:"version"`
	WorkspaceId    string            `json:"workspaceid"`
	ActiveTabId    string            `json:"activetabid"`
	ActiveBlockMap map[string]string `json:"activeblockmap"` // map from tabid to blockid
	Pos            Point             `json:"pos"`
	WinSize        WinSize           `json:"winsize"`
	LastFocusTs    int64             `json:"lastfocusts"`
}

func (*Window) GetOType() string {
	return "window"
}

type Workspace struct {
	OID     string   `json:"oid"`
	Version int      `json:"version"`
	Name    string   `json:"name"`
	TabIds  []string `json:"tabids"`
}

func (*Workspace) GetOType() string {
	return "workspace"
}

type Tab struct {
	OID      string   `json:"oid"`
	Version  int      `json:"version"`
	Name     string   `json:"name"`
	BlockIds []string `json:"blockids"`
}

func (*Tab) GetOType() string {
	return "tab"
}

type FileDef struct {
	FileType string         `json:"filetype,omitempty"`
	Path     string         `json:"path,omitempty"`
	Url      string         `json:"url,omitempty"`
	Content  string         `json:"content,omitempty"`
	Meta     map[string]any `json:"meta,omitempty"`
}

type BlockDef struct {
	Controller string              `json:"controller,omitempty"`
	View       string              `json:"view,omitempty"`
	Files      map[string]*FileDef `json:"files,omitempty"`
	Meta       map[string]any      `json:"meta,omitempty"`
}

type RuntimeOpts struct {
	TermSize shellexec.TermSize `json:"termsize,omitempty"`
	WinSize  WinSize            `json:"winsize,omitempty"`
}

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type WinSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Block struct {
	OID         string         `json:"oid"`
	Version     int            `json:"version"`
	BlockDef    *BlockDef      `json:"blockdef"`
	Controller  string         `json:"controller"`
	View        string         `json:"view"`
	Meta        map[string]any `json:"meta,omitempty"`
	RuntimeOpts *RuntimeOpts   `json:"runtimeopts,omitempty"`
}

func (*Block) GetOType() string {
	return "block"
}

func CreateTab(ctx context.Context, workspaceId string, name string) (*Tab, error) {
	return WithTxRtn(ctx, func(tx *TxWrap) (*Tab, error) {
		ws, _ := DBGet[*Workspace](tx.Context(), workspaceId)
		if ws == nil {
			return nil, fmt.Errorf("workspace not found: %q", workspaceId)
		}
		tab := &Tab{
			OID:      uuid.New().String(),
			Name:     name,
			BlockIds: []string{},
		}
		ws.TabIds = append(ws.TabIds, tab.OID)
		DBInsert(tx.Context(), tab)
		DBUpdate(tx.Context(), ws)
		return tab, nil
	})
}

func CreateWorkspace(ctx context.Context) (*Workspace, error) {
	ws := &Workspace{
		OID:    uuid.New().String(),
		TabIds: []string{},
	}
	DBInsert(ctx, ws)
	return ws, nil
}

func SetActiveTab(ctx context.Context, windowId string, tabId string) error {
	return WithTx(ctx, func(tx *TxWrap) error {
		window, _ := DBGet[*Window](tx.Context(), windowId)
		if window == nil {
			return fmt.Errorf("window not found: %q", windowId)
		}
		tab, _ := DBGet[*Tab](tx.Context(), tabId)
		if tab == nil {
			return fmt.Errorf("tab not found: %q", tabId)
		}
		window.ActiveTabId = tabId
		DBUpdate(tx.Context(), window)
		return nil
	})
}

func CreateBlock(ctx context.Context, tabId string, blockDef *BlockDef, rtOpts *RuntimeOpts) (*Block, error) {
	return WithTxRtn(ctx, func(tx *TxWrap) (*Block, error) {
		tab, _ := DBGet[*Tab](tx.Context(), tabId)
		if tab == nil {
			return nil, fmt.Errorf("tab not found: %q", tabId)
		}
		blockId := uuid.New().String()
		blockData := &Block{
			OID:         blockId,
			BlockDef:    blockDef,
			Controller:  blockDef.Controller,
			View:        blockDef.View,
			RuntimeOpts: rtOpts,
			Meta:        blockDef.Meta,
		}
		DBInsert(tx.Context(), blockData)
		tab.BlockIds = append(tab.BlockIds, blockId)
		DBUpdate(tx.Context(), tab)
		return blockData, nil
	})
}

func EnsureInitialData() error {
	// does not need to run in a transaction since it is called on startup
	ctx, cancelFn := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFn()
	clientCount, err := DBGetCount[*Client](ctx)
	if err != nil {
		return fmt.Errorf("error getting client count: %w", err)
	}
	if clientCount > 0 {
		return nil
	}
	windowId := uuid.New().String()
	workspaceId := uuid.New().String()
	tabId := uuid.New().String()
	client := &Client{
		OID:          uuid.New().String(),
		MainWindowId: windowId,
	}
	err = DBInsert(ctx, client)
	if err != nil {
		return fmt.Errorf("error inserting client: %w", err)
	}
	window := &Window{
		OID:            windowId,
		WorkspaceId:    workspaceId,
		ActiveTabId:    tabId,
		ActiveBlockMap: make(map[string]string),
		Pos: Point{
			X: 100,
			Y: 100,
		},
		WinSize: WinSize{
			Width:  800,
			Height: 600,
		},
	}
	err = DBInsert(ctx, window)
	if err != nil {
		return fmt.Errorf("error inserting window: %w", err)
	}
	ws := &Workspace{
		OID:    workspaceId,
		Name:   "default",
		TabIds: []string{tabId},
	}
	err = DBInsert(ctx, ws)
	if err != nil {
		return fmt.Errorf("error inserting workspace: %w", err)
	}
	tab := &Tab{
		OID:      tabId,
		Name:     "Tab-1",
		BlockIds: []string{},
	}
	err = DBInsert(ctx, tab)
	if err != nil {
		return fmt.Errorf("error inserting tab: %w", err)
	}
	return nil
}
