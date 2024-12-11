// Copyright (c) ClaceIO, LLC
// SPDX-License-Identifier: Apache-2.0

package action

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/benbjohnson/hashfs"
	"github.com/claceio/clace/internal/app/appfs"
	"github.com/claceio/clace/internal/app/apptype"
	"github.com/claceio/clace/internal/app/starlark_type"
	"github.com/claceio/clace/internal/system"
	"github.com/claceio/clace/internal/types"
	"github.com/go-chi/chi"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

//go:embed *.go.html astatic/*
var embedHtml embed.FS
var embedFS = hashfs.NewFS(embedHtml)

type ActionLink struct {
	Name string
	Path string
}

// Action represents a single action that is exposed by the App. Actions
// provide a way to trigger app operations, with an auto-generated form UI
// and an API interface
type Action struct {
	*types.Logger
	isDev             bool
	name              string
	description       string
	appPath           string
	run               starlark.Callable
	suggest           starlark.Callable
	params            []apptype.AppParam
	paramValuesStr    map[string]string
	paramDict         starlark.StringDict
	actionTemplate    *template.Template
	pagePath          string
	AppTemplate       *template.Template
	StyleType         types.StyleType
	LightTheme        string
	DarkTheme         string
	containerProxyUrl string
	hidden            map[string]bool // params which are not shown in the UI
	Links             []ActionLink    // links to other actions
	showValidate      bool
	auditInsert       func(*types.AuditEvent) error
}

// NewAction creates a new action
func NewAction(logger *types.Logger, sourceFS *appfs.SourceFs, isDev bool, name, description, apath string, run, suggest starlark.Callable,
	params []apptype.AppParam, paramValuesStr map[string]string, paramDict starlark.StringDict,
	appPath string, styleType types.StyleType, containerProxyUrl string, hidden []string, showValidate bool,
	auditInsert func(*types.AuditEvent) error) (*Action, error) {

	funcMap := system.GetFuncMap()

	funcMap["static"] = func(name string) string {
		fullPath := path.Join(appPath, sourceFS.HashName(name))
		return fullPath
	}

	funcMap["astatic"] = func(name string) string {
		fullPath := path.Join(appPath, embedFS.HashName(name))
		return fullPath
	}

	funcMap["fileNonEmpty"] = func(name string) bool {
		fi, err := sourceFS.Stat(name)
		if err != nil {
			return false
		}
		return fi.Size() > 0
	}

	tmpl, err := template.New("form").Funcs(funcMap).ParseFS(embedFS, "*.go.html")
	if err != nil {
		return nil, err
	}

	slices.SortFunc(params, func(a, b apptype.AppParam) int {
		return a.Index - b.Index
	})

	subLogger := logger.With().Str("action", name).Logger()
	appLogger := types.Logger{Logger: &subLogger}

	pagePath := path.Join(appPath, apath)
	if pagePath == "/" {
		pagePath = ""
	}

	hiddenParams := make(map[string]bool)
	for _, h := range hidden {
		hiddenParams[h] = true
	}

	return &Action{
		Logger:            &appLogger,
		isDev:             isDev,
		name:              name,
		description:       description,
		appPath:           appPath,
		pagePath:          pagePath,
		run:               run,
		suggest:           suggest,
		params:            params,
		paramValuesStr:    paramValuesStr,
		paramDict:         paramDict,
		actionTemplate:    tmpl,
		StyleType:         styleType,
		containerProxyUrl: containerProxyUrl,
		hidden:            hiddenParams,
		showValidate:      showValidate,
		auditInsert:       auditInsert,
		// Links, AppTemplate and Theme names are initialized later
	}, nil
}

func (a *Action) GetLink() ActionLink {
	return ActionLink{
		Name: a.name,
		Path: a.pagePath,
	}
}

// GetEmbeddedTemplates returns the embedded templates files
func GetEmbeddedTemplates() (map[string][]byte, error) {
	files, err := fs.Glob(embedFS, "*.go.html")
	if err != nil {
		return nil, err
	}

	templates := make(map[string][]byte)
	for _, file := range files {
		data, err := embedHtml.ReadFile(file)
		if err != nil {
			return nil, err
		}
		templates[file] = data
	}

	return templates, nil
}

func (a *Action) BuildRouter() (*chi.Mux, error) {
	r := chi.NewRouter()
	r.Get("/", a.getForm)
	r.Post("/", a.runAction)
	r.Post("/suggest", a.suggestAction)
	r.Post("/validate", a.validateAction)

	r.Handle("/astatic/*", http.StripPrefix(path.Join(a.pagePath), hashfs.FileServer(embedFS)))
	return r, nil
}

func (a *Action) runAction(w http.ResponseWriter, r *http.Request) {
	a.execAction(w, r, false, false, "execute")
}

func (a *Action) suggestAction(w http.ResponseWriter, r *http.Request) {
	a.execAction(w, r, true, false, "suggest")
}

func (a *Action) validateAction(w http.ResponseWriter, r *http.Request) {
	a.execAction(w, r, false, true, "validate")
}

func (a *Action) execAction(w http.ResponseWriter, r *http.Request, isSuggest, isValidate bool, op string) {
	if isSuggest && a.suggest == nil {
		http.Error(w, "suggest not supported for this action", http.StatusNotImplemented)
		return
	}

	thread := &starlark.Thread{
		Name:  a.name,
		Print: func(_ *starlark.Thread, msg string) { fmt.Println(msg) },
	}

	event := types.AuditEvent{
		RequestId:  system.GetContextRequestId(r.Context()),
		CreateTime: time.Now(),
		UserId:     system.GetContextUserId(r.Context()),
		EventType:  types.EventTypeAction,
		Operation:  op,
		Target:     a.name,
		Status:     "Success",
	}

	customEvent := types.AuditEvent{
		RequestId:  system.GetContextUserId(r.Context()),
		CreateTime: time.Now(),
		UserId:     system.GetContextUserId(r.Context()),
		EventType:  types.EventTypeCustom,
		Status:     "Success",
	}

	if a.auditInsert != nil {
		defer func() {
			if err := a.auditInsert(&event); err != nil {
				a.Error().Err(err).Msg("error inserting audit event")
			}

			op := system.GetThreadLocalKey(thread, types.TL_AUDIT_OPERATION)
			target := system.GetThreadLocalKey(thread, types.TL_AUDIT_TARGET)
			detail := system.GetThreadLocalKey(thread, types.TL_AUDIT_DETAIL)

			if op != "" {
				// Audit event was set, insert it
				customEvent.Operation = op
				customEvent.Target = target
				customEvent.Detail = detail
				if err := a.auditInsert(&customEvent); err != nil {
					a.Error().Err(err).Msg("error inserting custom audit event")
				}
			}
		}()
	}

	// Save the request context in the starlark thread local
	thread.SetLocal(types.TL_CONTEXT, r.Context())
	if a.containerProxyUrl != "" {
		thread.SetLocal(types.TL_CONTAINER_URL, a.containerProxyUrl)
	}
	isHtmxRequest := r.Header.Get("HX-Request") == "true"

	r.ParseMultipartForm(10 << 20) // 10 MB max file size
	var err error
	deferredCleanup := func() error {
		// Check for any deferred cleanups
		err = RunDeferredCleanup(thread)
		if err != nil {
			a.Error().Err(err).Msg("error cleaning up plugins")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return err
		}
		return nil
	}

	defer deferredCleanup()

	args := starlark.StringDict{}
	// Make a copy of the app level param dict
	for k, v := range a.paramDict {
		args[k] = v
	}

	qsParams := url.Values{}

	var tempDir string
	// Update args with submitted form values
	for _, param := range a.params {
		if a.hidden[param.Name] {
			continue
		}

		if param.DisplayType == apptype.DisplayTypeFileUpload {
			f, fh, err := r.FormFile(param.Name)
			if err == http.ErrMissingFile {
				args[param.Name] = starlark.String("")
				continue
			}

			if err != nil {
				http.Error(w, fmt.Sprintf("error getting file %s: %s", param.Name, err), http.StatusBadRequest)
				return
			}

			if tempDir == "" {
				tempDir, err = os.MkdirTemp("", "clace-file-upload-*")
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				defer func() {
					if remErr := os.RemoveAll(tempDir); remErr != nil {
						a.Error().Err(remErr).Msg("error removing temp dir")
					}
				}()
			}

			fullPath := path.Join(tempDir, fh.Filename)
			destFile, err := os.Create(fullPath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer destFile.Close()

			// Write contents of uploaded file to destFile
			if _, err = io.Copy(destFile, f); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			args[param.Name] = starlark.String(fullPath)
		} else {
			// Not file upload, regular param
			formValue := r.Form.Get(param.Name)
			if formValue == "" {
				if param.Type == starlark_type.BOOLEAN {
					// Form does not submit unchecked checkboxes, set to false
					args[param.Name] = starlark.Bool(false)
					qsParams.Add(param.Name, "false")
				}
			} else {
				newVal, err := apptype.ParamStringToType(param.Name, param.Type, formValue)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				args[param.Name] = newVal

				if param.DisplayType != apptype.DisplayTypePassword {
					qsParams.Add(param.Name, formValue)
				}
			}
		}
	}

	argsValue := Args{members: args}

	callable := a.run
	callInput := starlark.Tuple{starlark.Bool(isValidate), &argsValue}
	if isSuggest {
		callable = a.suggest
		callInput = starlark.Tuple{&argsValue}
	}

	// Call the handler function
	var ret starlark.Value
	ret, err = starlark.Call(thread, callable, callInput, nil)

	if err == nil {
		pluginErrLocal := thread.Local(types.TL_PLUGIN_API_FAILED_ERROR)
		if pluginErrLocal != nil {
			pluginErr := pluginErrLocal.(error)
			a.Error().Err(pluginErr).Msg("handler had plugin API failure")
			err = pluginErr // handle as if the handler had returned an error
		}
	}

	if err != nil {
		event.Status = "Error"
		a.Error().Err(err).Msg("error calling action run handler")

		firstFrame := ""
		if evalErr, ok := err.(*starlark.EvalError); ok {
			// Iterate through the CallFrame stack for debugging information
			for i, frame := range evalErr.CallStack {
				a.Warn().Msgf("Function: %s, Position: %s\n", frame.Name, frame.Pos)
				if i == 0 {
					firstFrame = fmt.Sprintf("Function %s, Position %s", frame.Name, frame.Pos)
				}
			}
		}

		msg := err.Error()
		if firstFrame != "" && a.isDev {
			msg = msg + " : " + firstFrame
		}

		// err handler is not supported for actions
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	if isSuggest {
		a.handleSuggestResponse(w, ret)
		return
	}

	var valuesMap []map[string]any
	var valuesStr []string
	var status string
	var paramErrors map[string]any
	report := apptype.AUTO

	resultStruct, ok := ret.(*starlarkstruct.Struct)
	if ok {
		status, err = apptype.GetOptionalStringAttr(resultStruct, "status")
		if err != nil {
			http.Error(w, fmt.Sprintf("error getting result status: %s", err), http.StatusInternalServerError)
			return
		}

		valuesMap, err = apptype.GetListMapAttr(resultStruct, "values", true)
		if err != nil {
			valuesStr, err = apptype.GetListStringAttr(resultStruct, "values", true)
			if err != nil {
				http.Error(w, fmt.Sprintf("error getting result values, not a list of string or list of maps: %s", err), http.StatusInternalServerError)
				return
			}
		}

		paramErrors, err = apptype.GetDictAttr(resultStruct, "param_errors", true)
		if err != nil {
			http.Error(w, fmt.Sprintf("error getting result attr paramErrors: %s", err), http.StatusInternalServerError)
			return
		}

		report, err = apptype.GetOptionalStringAttr(resultStruct, "report")
		if err != nil {
			http.Error(w, fmt.Sprintf("error getting result report: %s", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Not a result struct
		status = strings.Trim(ret.String(), "\"")
	}

	if deferredCleanup() != nil {
		return
	}

	if err != nil {
		event.Status = "Error"
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pageInput := map[string]any{
		"name":        a.name,
		"description": a.description,
		"path":        a.pagePath,
		"lightTheme":  a.LightTheme,
		"darkTheme":   a.DarkTheme,
	}

	if !isHtmxRequest {
		err = a.actionTemplate.ExecuteTemplate(w, "header", pageInput)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Set the push URL for HTMX
		w.Header().Set("HX-Push-Url", a.pagePath+"?"+qsParams.Encode())
	}

	// Render the result message
	err = a.actionTemplate.ExecuteTemplate(w, "status", status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Render the param error messages, using HTMX OOB
	errorMsgs := map[string]string{}
	errorKeys := []string{}
	for _, param := range a.params {
		// "" error messages have to be sent to overwrite previous values in form UI
		if !strings.HasPrefix(param.Name, OPTIONS_PREFIX) {
			if paramErrors[param.Name] == nil {
				errorMsgs[param.Name] = ""
			} else {
				errorMsgs[param.Name] = fmt.Sprintf("%s", paramErrors[param.Name])
			}
			errorKeys = append(errorKeys, param.Name)
		}
	}

	slices.Sort(errorKeys)
	for _, paramName := range errorKeys {
		tv := struct {
			Name    string
			Message string
		}{
			Name:    paramName,
			Message: errorMsgs[paramName],
		}
		err = a.actionTemplate.ExecuteTemplate(w, "paramError", tv)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if isValidate {
		// No need to render the results
		return
	}

	err = a.renderResults(w, report, valuesMap, valuesStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !isHtmxRequest {
		err = a.actionTemplate.ExecuteTemplate(w, "footer", pageInput)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func (a *Action) renderResults(w http.ResponseWriter, report string, valuesMap []map[string]any, valuesStr []string) error {
	if report == apptype.AUTO {
		return a.renderResultsAuto(w, valuesMap, valuesStr)
	}

	switch report {
	case apptype.TABLE:
		return a.renderResultsTable(w, valuesMap)
	case apptype.TEXT:
		return a.renderResultsText(w, valuesStr)
	case apptype.JSON:
		return a.renderResultsJson(w, valuesMap)
	case apptype.DOWNLOAD:
		return a.renderResultsDownload(w, valuesMap)
	case apptype.IMAGE:
		return a.renderResultsImage(w, valuesMap)
	default:
		// Custom template being used for the results
		// Wrap the template output in a div with hx-swap-oob
		_, err := io.WriteString(w, `<div id="action_result" hx-swap-oob="true" hx-swap="outerHTML"> `)
		if err != nil {
			return err
		}
		var tmplErr error
		if len(valuesStr) > 0 {
			tmplErr = a.AppTemplate.ExecuteTemplate(w, report, valuesStr)
		} else {
			tmplErr = a.AppTemplate.ExecuteTemplate(w, report, valuesMap)
		}
		_, err = io.WriteString(w, ` </div>`)
		if err != nil {
			return err
		}

		return tmplErr
	}
}

func (a *Action) renderResultsAuto(w http.ResponseWriter, valuesMap []map[string]any, valuesStr []string) error {
	if len(valuesStr) > 0 {
		return a.renderResultsText(w, valuesStr)
	}

	if len(valuesMap) == 0 {
		return a.actionTemplate.ExecuteTemplate(w, "result-empty", nil)
	}

	if len(valuesMap) > 0 {
		firstRow := valuesMap[0]
		hasComplex := false
		for _, v := range firstRow {
			if v == nil {
				continue
			}
			switch v.(type) {
			case int:
			case string:
			case bool:
			default:
				hasComplex = true
			}
			if hasComplex {
				break
			}
		}

		if hasComplex {
			return a.renderResultsJson(w, valuesMap)
		}
		return a.renderResultsTable(w, valuesMap)
	}

	return nil
}

func (a *Action) renderResultsText(w http.ResponseWriter, valuesStr []string) error {
	// Render the result values, using HTMX OOB
	err := a.actionTemplate.ExecuteTemplate(w, "result-textarea", valuesStr)
	return err
}

func (a *Action) renderResultsDownload(w http.ResponseWriter, valuesMap []map[string]any) error {
	// Render the result values, using HTMX OOB
	err := a.actionTemplate.ExecuteTemplate(w, "result-download", valuesMap)
	return err
}

func (a *Action) renderResultsImage(w http.ResponseWriter, valuesMap []map[string]any) error {
	// Render the result values, using HTMX OOB
	err := a.actionTemplate.ExecuteTemplate(w, "result-image", valuesMap)
	return err
}

func (a *Action) renderResultsTable(w http.ResponseWriter, valuesMap []map[string]any) error {
	if len(valuesMap) == 0 {
		return a.actionTemplate.ExecuteTemplate(w, "result-empty", nil)
	}
	firstRow := valuesMap[0]
	keys := make([]string, 0, len(firstRow))
	for k := range firstRow {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	values := make([][]string, 0, len(valuesMap))
	for _, row := range valuesMap {
		rowValues := make([]string, 0, len(keys))
		for _, k := range keys {
			v, ok := row[k]
			if !ok {
				// Missing value
				rowValues = append(rowValues, "")
			} else {
				pv := fmt.Sprintf("%v", v)
				DISPLAY_LIMIT := 100
				if len(pv) > DISPLAY_LIMIT {
					pv = pv[:DISPLAY_LIMIT] + "..."
				}
				rowValues = append(rowValues, pv)
			}
		}
		values = append(values, rowValues)
	}

	input := map[string]any{
		"Keys":   keys,
		"Values": values,
	}

	err := a.actionTemplate.ExecuteTemplate(w, "result-table", input)
	return err
}

func (a *Action) renderResultsJson(w http.ResponseWriter, valuesMap []map[string]any) error {
	err := a.actionTemplate.ExecuteTemplate(w, "result-json", valuesMap)
	return err
}

func RunDeferredCleanup(thread *starlark.Thread) error {
	deferMap := thread.Local(types.TL_DEFER_MAP)
	if deferMap == nil {
		return nil
	}

	strictFailures := []string{}
	for pluginName, pluginMap := range deferMap.(map[string]map[string]apptype.DeferEntry) {
		for key, entry := range pluginMap {
			err := entry.Func()
			if err != nil {
				fmt.Printf("error cleaning up %s %s: %s\n", pluginName, key, err)
			}
			if entry.Strict {
				strictFailures = append(strictFailures, fmt.Sprintf("%s:%s", pluginName, key))
			}
		}
	}

	thread.SetLocal(types.TL_DEFER_MAP, nil) // reset the defer map

	if len(strictFailures) > 0 {
		return fmt.Errorf("resource has not be closed, check handler code: %s", strings.Join(strictFailures, ", "))
	}

	return nil
}

type ParamDef struct {
	Name               string
	Description        string
	Value              any
	InputType          string
	Options            []string
	DisplayType        string
	DisplayTypeOptions string
}

const (
	OPTIONS_PREFIX = "options-"
)

func (a *Action) getForm(w http.ResponseWriter, r *http.Request) {
	queryParams := r.URL.Query()
	params := make([]ParamDef, 0, len(a.params))

	options := make(map[string][]string)
	for _, p := range a.params {
		// params with options-x prefix are treated as select options for x
		if strings.HasPrefix(p.Name, OPTIONS_PREFIX) {
			name := p.Name[len(OPTIONS_PREFIX):]
			var vals []string
			err := json.Unmarshal([]byte(a.paramValuesStr[p.Name]), &vals)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid value for %s: %s", p.Name, a.paramValuesStr[p.Name]), http.StatusBadRequest)
				return
			}
			options[name] = vals
		}
	}

	hasFileUpload := false
	for _, p := range a.params {
		if strings.HasPrefix(p.Name, OPTIONS_PREFIX) || a.hidden[p.Name] {
			continue
		}

		param := ParamDef{
			Name:        p.Name,
			Description: p.Description,
		}

		value, ok := a.paramValuesStr[p.Name]
		if !ok {
			http.Error(w, fmt.Sprintf("missing param value for %s", p.Name), http.StatusInternalServerError)
			return
		}

		qValue := queryParams.Get(p.Name)
		if qValue != "" {
			// Prefer value from query params
			value = qValue
		}

		param.Value = value // Default to string format
		param.InputType = "text"
		if p.Type == starlark_type.BOOLEAN {
			boolValue, err := strconv.ParseBool(value)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid value for %s: %s", p.Name, value), http.StatusInternalServerError)
				return
			}
			if boolValue {
				param.Value = "checked"
			}
			param.InputType = "checkbox"
		} else if options[p.Name] != nil {
			param.InputType = "select"
			param.Options = options[p.Name]
			param.Value = value
		}

		if p.DisplayType != "" {
			switch p.DisplayType {
			case apptype.DisplayTypePassword:
				param.DisplayType = "password"
			case apptype.DisplayTypeTextArea:
				param.DisplayType = "textarea"
			case apptype.DisplayTypeFileUpload:
				param.DisplayType = "file"
				hasFileUpload = true
			default:
				http.Error(w, fmt.Sprintf("invalid display type for %s: %s", p.Name, p.DisplayType), http.StatusInternalServerError)
				return
			}
			param.DisplayTypeOptions = p.DisplayTypeOptions
		} else {
			param.DisplayType = "text"
		}

		params = append(params, param)
	}

	linksWithQS := make([]ActionLink, 0, len(a.Links))
	for _, link := range a.Links {
		if link.Path != a.pagePath { // Don't add self link
			if r.URL.RawQuery != "" {
				link.Path = link.Path + "?" + r.URL.RawQuery
			}
			linksWithQS = append(linksWithQS, link)
		}
	}

	input := map[string]any{
		"dev":           a.isDev,
		"name":          a.name,
		"description":   a.description,
		"appPath":       a.appPath,
		"pagePath":      a.pagePath,
		"params":        params,
		"styleType":     string(a.StyleType),
		"lightTheme":    a.LightTheme,
		"darkTheme":     a.DarkTheme,
		"links":         linksWithQS,
		"hasFileUpload": hasFileUpload,
		"showSuggest":   a.suggest != nil,
		"showValidate":  a.showValidate,
	}
	err := a.actionTemplate.ExecuteTemplate(w, "form.go.html", input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *Action) handleSuggestResponse(w http.ResponseWriter, retVal starlark.Value) {
	ret, err := starlark_type.UnmarshalStarlark(retVal)
	if err != nil {
		http.Error(w, fmt.Sprintf("error unmarshalling suggest response: %s", err), http.StatusInternalServerError)
		return
	}

	message, retIsString := ret.(string)
	if !retIsString {
		message = "Suggesting values"
	}

	err = a.actionTemplate.ExecuteTemplate(w, "status", message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if retIsString {
		// No suggestions available
		return
	}

	retDict := map[string]any{}
	switch retType := ret.(type) {
	case map[string]any:
		for k, v := range retType {
			retDict[k] = v
		}
	case map[string]string:
		for k, v := range retType {
			retDict[k] = v
		}
	case map[string]int:
		for k, v := range retType {
			retDict[k] = v
		}
	case map[string]bool:
		for k, v := range retType {
			retDict[k] = v
		}
	case map[string][]string:
		for k, v := range retType {
			retDict[k] = v
		}
	default:
		http.Error(w, fmt.Sprintf("invalid suggest response type: %T, expected dict", retType), http.StatusInternalServerError)
		return
	}

	paramMap := map[string]apptype.AppParam{}
	for _, p := range a.params {
		paramMap[p.Name] = p
	}

	keys := slices.Collect(maps.Keys(retDict))
	slices.Sort(keys)
	for _, key := range keys {
		value := retDict[key]
		p, ok := paramMap[key]
		if !ok || strings.HasPrefix(key, OPTIONS_PREFIX) {
			a.Info().Msgf("ignoring suggest response for param: %s", key)
			continue
		}
		param := ParamDef{
			Name:        p.Name,
			Description: p.Description,
		}

		param.Value = fmt.Sprintf("%v", value)
		param.InputType = "text"

		valueList, valueIsList := value.([]string)
		if p.DisplayType == apptype.DisplayTypeFileUpload {
			http.Error(w, fmt.Sprintf("suggest not supported for file upload param: %s", p.Name), http.StatusInternalServerError)
			return
		} else if p.Type == starlark_type.STRING && valueIsList {
			param.InputType = "select"
			param.Value = valueList[0]
			param.Options = valueList
		} else if p.Type == starlark_type.BOOLEAN {
			boolValue, err := strconv.ParseBool(fmt.Sprintf("%v", value))
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid value for %s: %s", p.Name, value), http.StatusInternalServerError)
				return
			}
			if boolValue {
				param.Value = "checked"
			}
			param.InputType = "checkbox"
		}

		if p.DisplayType != "" {
			switch p.DisplayType {
			case apptype.DisplayTypePassword:
				param.DisplayType = "password"
			case apptype.DisplayTypeTextArea:
				param.DisplayType = "textarea"
			case apptype.DisplayTypeFileUpload:
				param.DisplayType = "file"
			default:
				http.Error(w, fmt.Sprintf("invalid display type for %s: %s", p.Name, p.DisplayType), http.StatusInternalServerError)
				return
			}
			param.DisplayTypeOptions = p.DisplayTypeOptions
		} else {
			param.DisplayType = "text"
		}

		err = a.actionTemplate.ExecuteTemplate(w, "param_suggest", param)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
