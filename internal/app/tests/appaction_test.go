package app_test

import (
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"testing"

	"github.com/claceio/clace/internal/app"
	"github.com/claceio/clace/internal/testutil"
)

func actionTester(t *testing.T, rootPath bool, actionPath string) {
	t.Helper()
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=["a", "b"], report=ace.TEXT)

app = ace.app("testApp",
	actions=[ace.action("testAction", "` + actionPath + `", handler)])

		`,
		"params.star": `param("param1", description="param1 description", type=STRING, default="myvalue")`,
	}
	var a *app.App
	var err error
	if rootPath {
		a, _, err = CreateTestAppRoot(logger, fileData)
	} else {
		a, _, err = CreateTestApp(logger, fileData)
	}
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	var reqPath string
	if rootPath {
		reqPath = actionPath
	} else {
		reqPath = path.Join("/test", actionPath)
	}

	request := httptest.NewRequest("GET", reqPath, nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "<title>testAction</title>")
	testutil.AssertStringContains(t, response.Body.String(), `id="param_param1"`)

	request = httptest.NewRequest("POST", reqPath, nil)
	request.Header.Set("HX-Request", "true")
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringMatch(t, "match response", response.Body.String(), `
          <div class="text-lg text-bold">
            done
          </div>

		 <div
			id="param_param1_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		 </div>
        
          <div id="action_result" hx-swap-oob="true" hx-swap="outerHTML">
            <div class="divider text-lg text-secondary">Output</div>
            <textarea
              rows="20"
              class="textarea textarea-success w-full font-mono"
              readonly>a
        b
        </textarea
            >
          </div>
        `)

}

func TestRootAppRootAction(t *testing.T) {
	actionTester(t, true, "/")
}

func TestRootApp(t *testing.T) {
	actionTester(t, true, "/abc")
}

func TestNonRootAppRootAction(t *testing.T) {
	actionTester(t, false, "/")
}

func TestNonRootApp(t *testing.T) {
	actionTester(t, false, "/abc")
}

func TestParamErrors(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=["a", "b"], param_errors={"param1": "param1error", "param3": "param3error"})

app = ace.app("testApp", 
	actions=[ace.action("testAction", "/", handler)])

		`,
		"params.star": `param("param1", description="param1 description", type=STRING, default="myvalue")
param("param2", description="param2 description", type=BOOLEAN, default=True)
param("param3", description="param3 description", type=INT, default=10)`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "<title>testAction</title>")
	testutil.AssertStringContains(t, response.Body.String(), `id="param_param1"`)

	request = httptest.NewRequest("POST", "/test", nil)
	request.Header.Set("HX-Request", "true")
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringMatch(t, "match response", `
	<div class="text-lg text-bold">
            done
          </div>
        
          <div
            id="param_param1_error"
            hx-swap-oob="true"
            hx-swap="outerHTML"
            class="text-error mt-1">
            param1error
          </div>

		  <div
			id="param_param2_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		  </div>

          <div
            id="param_param3_error"
            hx-swap-oob="true"
            hx-swap="outerHTML"
            class="text-error mt-1">
            param3error
          </div>
        
          <div id="action_result" hx-swap-oob="true" hx-swap="outerHTML">
            <div class="divider text-lg text-secondary">Output</div>
            <textarea
              rows="20"
              class="textarea textarea-success w-full font-mono"
              readonly>a
        b
        </textarea >
          </div>
        `, response.Body.String())
}

func TestAutoReportTable(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=[{"a": 1, "b": "abc"}])

app = ace.app("testApp",
	actions=[ace.action("testAction", "/", handler)])

		`,
		"params.star": `param("param1", description="param1 description", type=STRING, default="myvalue")`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "<title>testAction</title>")
	testutil.AssertStringContains(t, response.Body.String(), `id="param_param1"`)

	request = httptest.NewRequest("POST", "/test", nil)
	request.Header.Set("HX-Request", "true")
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringMatch(t, "match response", `
	<div class="text-lg text-bold"> done </div>

		 <div
			id="param_param1_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		</div>
        
            <div id="action_result" hx-swap-oob="true" hx-swap="outerHTML">
            <div class="divider text-lg text-secondary">Report</div>
        
            <table class="table table-auto min-w-full table-zebra text-xl font-mono">
              <thead>
                <tr>
                    <th>a</th>
                    <th>b</th>
                </tr>
              </thead>
              <tbody>
                  <tr>
                      <td>1</td>
                      <td>abc</td>
                  </tr>
              </tbody>
            </table>
          </div>`, response.Body.String())
}

func TestAutoReportJSON(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=[{"a": {"c": 1}, "b": "abc"}], report=ace.AUTO)

app = ace.app("testApp",
	actions=[ace.action("testAction", "/", handler)])

		`,
		"params.star": `param("param1", description="param1 description", type=STRING, default="myvalue")`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "<title>testAction</title>")
	testutil.AssertStringContains(t, response.Body.String(), `id="param_param1"`)

	request = httptest.NewRequest("POST", "/test", nil)
	request.Header.Set("HX-Request", "true")
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringMatch(t, "match response", `
		  <div class="text-lg text-bold"> done </div>

		 <div
			id="param_param1_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		 </div>
        
          <div id="action_result" hx-swap-oob="true" hx-swap="outerHTML">
            <div class="divider text-lg text-secondary">Result</div>
            <div class="json-container" data-json="{&#34;a&#34;:{&#34;c&#34;:1},&#34;b&#34;:&#34;abc&#34;}"></div>
            <script>
              document.querySelectorAll(".json-container").forEach(function (div) {
                const jsonData = JSON.parse(div.getAttribute("data-json"));
                renderJSONWithRoot(jsonData, div);
              });
            </script>
          </div>`, response.Body.String())
}

func TestReportTable(t *testing.T) {
	// Force table format for output containing map
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=[{"a": {"c": 1}, "b": "abc"}], report=ace.TABLE)

app = ace.app("testApp",
	actions=[ace.action("testAction", "/", handler)])

		`,
		"params.star": `param("param1", description="param1 description", type=STRING, default="myvalue")`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "<title>testAction</title>")
	testutil.AssertStringContains(t, response.Body.String(), `id="param_param1"`)

	request = httptest.NewRequest("POST", "/test", nil)
	request.Header.Set("HX-Request", "true")
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringMatch(t, "match response", `
	<div class="text-lg text-bold">
            done
          </div>

		 <div
			id="param_param1_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		 </div>
        
          <div id="action_result" hx-swap-oob="true" hx-swap="outerHTML">
            <div class="divider text-lg text-secondary">Report</div>
        
            <table class="table table-auto min-w-full table-zebra text-xl font-mono">
              <thead>
                <tr>
                    <th>a</th>
                    <th>b</th>
                </tr>
              </thead>
              <tbody>
                  <tr>
                      <td>map[c:1]</td>
                      <td>abc</td>
                  </tr>
              </tbody>
            </table>
          </div>`, response.Body.String())
}

func TestReportTableMissingData(t *testing.T) {
	// Force table format for output containing map
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=[{"a": 1, "b": "abc"}, {"c": 1, "b": "abc2"}], report=ace.TABLE)

app = ace.app("testApp",
	actions=[ace.action("testAction", "/", handler)])

		`,
		"params.star": `param("param1", description="param1 description", type=STRING, default="myvalue")`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "<title>testAction</title>")
	testutil.AssertStringContains(t, response.Body.String(), `id="param_param1"`)

	request = httptest.NewRequest("POST", "/test", nil)
	request.Header.Set("HX-Request", "true")
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringMatch(t, "match response", `<div class="text-lg text-bold">
            done
          </div>

		 <div
			id="param_param1_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		 </div>
        
          <div id="action_result" hx-swap-oob="true" hx-swap="outerHTML">
            <div class="divider text-lg text-secondary">Report</div>
        
            <table class="table table-auto min-w-full table-zebra text-xl font-mono">
              <thead>
                <tr>
                    <th>a</th>
                    <th>b</th>
                </tr>
              </thead>
              <tbody>
                  <tr>
                      <td>1</td>
                      <td>abc</td>
                  </tr>
                  <tr>
                      <td></td>
                      <td>abc2</td>
                  </tr>
              </tbody>
            </table>
          </div>`, response.Body.String())
}

func TestParamPost(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=[{"c1": args.param1, "c2": args.param2, "c3": args.param3}], report=ace.TABLE)

app = ace.app("testApp",
	actions=[ace.action("testAction", "/", handler)])

		`,
		"params.star": `param("param1", description="param1 description", type=STRING, default="myvalue")
param("param2", description="param2 description", type=BOOLEAN, default=False)
param("param3", description="param3 description", type=INT, default=10)`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "<title>testAction</title>")
	testutil.AssertStringContains(t, response.Body.String(), `id="param_param1"`)

	values := url.Values{
		"param1": {"abc"},
		"param2": {"true"},
		"param3": {"20"},
	}

	request = httptest.NewRequest("POST", "/test", strings.NewReader(values.Encode()))
	request.Header.Set("HX-Request", "true")
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertEqualsString(t, "push url", "/test?param1=abc&param2=true&param3=20", response.Header().Get("HX-Push-Url"))
	testutil.AssertStringMatch(t, "match response", `
	<div class="text-lg text-bold">
            done
          </div>

		 <div
			id="param_param1_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		 </div>

		 <div
			id="param_param2_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		 </div>

		 <div
			id="param_param3_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		 </div>
        
          <div id="action_result" hx-swap-oob="true" hx-swap="outerHTML">
            <div class="divider text-lg text-secondary">Report</div>
        
            <table class="table table-auto min-w-full table-zebra text-xl font-mono">
              <thead>
                <tr>
                    <th>c1</th>
                    <th>c2</th>
                    <th>c3</th>
                </tr>
              </thead>
              <tbody>
                  <tr>
                      <td>abc</td>
                      <td>true</td>
                      <td>20</td>
                  </tr>
              </tbody>
            </table>
          </div>`, response.Body.String())
}

func TestCustomReport(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=[{"a": 1, "b": "abc"}], report="custom")

app = ace.app("testApp",
	actions=[ace.action("testAction", "/", handler)])

		`,
		"params.star":    `param("param1", description="param1 description", type=STRING, default="myvalue")`,
		"myfile.go.html": `{{block "custom" .}} customdata {{end}}`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "<title>testAction</title>")
	testutil.AssertStringContains(t, response.Body.String(), `id="param_param1"`)

	request = httptest.NewRequest("POST", "/test", nil)
	request.Header.Set("HX-Request", "true")
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringMatch(t, "match response", `<div class="text-lg text-bold">
            done
          </div>

		<div
			id="param_param1_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		</div>

        <div id="action_result" hx-swap-oob="true" hx-swap="outerHTML">  customdata  </div>`, response.Body.String())

	// Unset the template
	fileData["myfile.go.html"] = ``
	a, _, err = CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}
	request = httptest.NewRequest("GET", "/test/", nil)
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "<title>testAction</title>")
	testutil.AssertStringContains(t, response.Body.String(), `id="param_param1"`)

	request = httptest.NewRequest("POST", "/test", nil)
	request.Header.Set("HX-Request", "true")
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringMatch(t, "match response", `<div class="text-lg text-bold"> done </div>
		<div
			id="param_param1_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		</div>
        <div id="action_result" hx-swap-oob="true" hx-swap="outerHTML">  </div>html/template: "custom" is undefined`, response.Body.String())
}

func TestParamOptions(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=["a", "b"], param_errors={"param1": "param1error", "param3": "param3error"})

app = ace.app("testApp",
	actions=[ace.action("testAction", "/", handler, hidden=["param2"])])

		`,
		"params.star": `param("param1", description="param1 description", type=STRING, default="myvalue")
param("options-param1", description="param1 options", type=LIST, default=["a", "b", "c"])
param("param2", description="param2 description", type=STRING, default="myvalue2")`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "<title>testAction</title>")
	bodyStripped := strings.Join(strings.Fields(response.Body.String()), " ")
	testutil.AssertStringContains(t, bodyStripped, `select id="param_param1`)
	if strings.Contains(bodyStripped, `options-param1`) {
		t.Errorf("options-param1 should not be in the body")
	}
	if strings.Contains(bodyStripped, `param2`) {
		t.Errorf("hidden param2 should not be in the body")
	}
}

func TestActionError(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	if args.param1 == "error":
		return "errormessage"
	10/args.param3 
	return ace.result(status="done", values=[{"c1": args.param1, "c2": args.param2, "c3": args.param3}], report=ace.TABLE)

app = ace.app("testApp",
	actions=[ace.action("testAction", "/", handler)])

		`,
		"params.star": `param("param1", description="param1 description", type=STRING, default="myvalue")
param("param2", description="param2 description", type=BOOLEAN, default=False)
param("param3", description="param3 description", type=INT, default=10)`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "<title>testAction</title>")
	testutil.AssertStringContains(t, response.Body.String(), `id="param_param1"`)

	values := url.Values{
		"param1": {"error"},
		"param2": {"true"},
		"param3": {"20"},
	}

	request = httptest.NewRequest("POST", "/test", strings.NewReader(values.Encode()))
	request.Header.Set("HX-Request", "true")
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringMatch(t, "match response", `<div class="text-lg text-bold">
            errormessage 
          </div>
		<div
			id="param_param1_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		</div>

		<div
			id="param_param2_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		</div>

		<div
			id="param_param3_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		</div>
		<div id="action_result" hx-swap-oob="true" hx-swap="outerHTML"> <div class="divider text-lg text-secondary">No Output</div> </div>
		`, response.Body.String())

	values = url.Values{
		"param1": {"p1val"},
		"param2": {"true"},
		"param3": {"0"},
	}

	request = httptest.NewRequest("POST", "/test", strings.NewReader(values.Encode()))
	request.Header.Set("HX-Request", "true")
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 500, response.Code)
	testutil.AssertStringMatch(t, "response", `floating-point division by zero`, response.Body.String())

	values = url.Values{
		"param1": {"p1val"},
		"param2": {"true"},
		"param3": {"50"},
	}

	request = httptest.NewRequest("POST", "/test", strings.NewReader(values.Encode()))
	request.Header.Set("HX-Request", "true")
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringMatch(t, "response", `<div class="text-lg text-bold">
            done
          </div>

		<div
			id="param_param1_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		</div>

		<div
			id="param_param2_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		</div>

		<div
			id="param_param3_error"
			hx-swap-oob="true"
			hx-swap="outerHTML"
			class="text-error mt-1">
  		</div>
        
          <div id="action_result" hx-swap-oob="true" hx-swap="outerHTML">
            <div class="divider text-lg text-secondary">Report</div>
        
            <table class="table table-auto min-w-full table-zebra text-xl font-mono">
              <thead>
                <tr>
                  
                    <th>c1</th>
                  
                    <th>c2</th>
                  
                    <th>c3</th>
                  
                </tr>
              </thead>
              <tbody>
                
                  <tr>
                    
                      <td>p1val</td>
                    
                      <td>true</td>
                    
                      <td>50</td>
                    
                  </tr>
                
              </tbody>
            </table>
          </div>`, response.Body.String())
}

func TestNonHtmxRequest(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=[{"a": 1, "b": "abc"}], report="custom")

app = ace.app("testApp",
	actions=[ace.action("testAction", "/", handler)])

		`,
		"params.star":    `param("param1", description="param1 description", type=STRING, default="myvalue")`,
		"myfile.go.html": `{{block "custom" .}} customdata {{end}}`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "<title>testAction</title>")
	testutil.AssertStringContains(t, response.Body.String(), `id="param_param1"`)

	request = httptest.NewRequest("POST", "/test", nil)
	// no header request.Header.Set("HX-Request", "true")
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	body := response.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Errorf("Expected full html response, got: %s", body)
	}
	if !strings.Contains(body, "</html>") {
		t.Errorf("Expected full html response, got: %s", body)
	}
}

func TestMultipleActions(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=[{"a": 1, "b": "abc"}])

app = ace.app("testApp",
	actions=[ace.action("test1Action", "/test1", handler),
	         ace.action("test2Action", "/test2", handler)])

		`,
		"params.star": `param("param1", description="param1 description", type=STRING, default="myvalue")`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/test1", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	body := response.Body.String()
	if strings.Contains(body, `<li><a href="/test/test1">test1Action</a></li>`) {
		t.Errorf("actions switcher should not have current action, got %s", body)
	}
	testutil.AssertStringContains(t, body, `<li><a href="/test/test2">test2Action</a></li>`)

	request = httptest.NewRequest("GET", "/test/test2?param1=abc", nil)
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	body = response.Body.String()
	if strings.Contains(body, `<li><a href="/test/test2">test2Action</a></li>`) {
		t.Errorf("actions switcher should not have current action, got %s", body)
	}
	testutil.AssertStringContains(t, body, `<li><a href="/test/test1?param1=abc">test1Action</a></li>`)
}

func TestDisplayTypes(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=[{"a": 1, "b": "abc"}])

app = ace.app("testApp",
	actions=[ace.action("test1Action", "/test1", handler)])
		`,
		"params.star": `param("param1", description="param1 description", default="myvalue", display_type=FILE)
param("param2", description="param2 description", default="myvalue", display_type=PASSWORD)
param("param3", description="param3 description", default="myvalue", display_type=TEXTAREA)`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/test1", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	body := response.Body.String()
	testutil.AssertStringContains(t, body, `type="file"`)
	testutil.AssertStringContains(t, body, `type="password"`)
	testutil.AssertStringContains(t, body, `textarea`)
}

func TestDisplayTypesError(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=[{"a": 1, "b": "abc"}])

app = ace.app("testApp",
	actions=[ace.action("test1Action", "/test1", handler)])
		`,
		"params.star": `param("param1", description="param1 description", type=BOOLEAN, default=False, display_type=FILE)`,
	}
	_, _, err := CreateTestApp(logger, fileData)
	testutil.AssertErrorContains(t, err, "display_type file is allowed for string type param1 only")
}

func TestSuggest(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
	return ace.result(status="done", values=[{"a": {"c": 1}, "b": "abc"}], report=ace.AUTO)

def suggest_handler(args):
	return {"param1": ["a", "b", "c"], "param2": True}

app = ace.app("testApp",
	actions=[ace.action("testAction", "/", handler, suggest=suggest_handler)])

		`,
		"params.star": `param("param1", description="param1 description", type=STRING, default="myvalue")
param("param2", description="param2 description", type=BOOLEAN, default=False)`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "/test/suggest")
	if strings.Contains(response.Body.String(), "/test/validate") {
		t.Errorf("validate API should not be in the body")
	}

	request = httptest.NewRequest("POST", "/test/suggest", nil)
	request.Header.Set("HX-Request", "true")
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringMatch(t, "body", `<div class="text-lg text-bold">
            Suggesting values
          </div>
        
          <div id="param_param1_div" hx-swap-oob="true" hx-swap="outerHTML">
            
                    
                      <div>
                        <select
                          id="param_param1"
                          class="select select-bordered w-full"
                          name="param1">
                          
                          
                            <option value="a" selected>
                              a
                            </option>
                          
                            <option value="b" >
                              b
                            </option>
                          
                            <option value="c" >
                              c
                            </option>
                          
                        </select>
                        <div id="param_param1_error" class="text-error mt-1"></div>
                      </div>
                    
                    
                  
          </div>
        
          <div id="param_param2_div" hx-swap-oob="true" hx-swap="outerHTML">
            
                    
                      <div class="flex justify-center">
                        <input
                          id="param_param2"
                          name="param2"
                          type="checkbox"
                          value="true"
                          class="checkbox checkbox-primary justify-self-center"
                          checked />
                        <div class="pl-4">
                          <div
                            id="param_param2_error"
                            class="text-error mt-1"></div>
                        </div>
                      </div>
                    
                    
                  
          </div>`, response.Body.String())
}

func TestValidate(t *testing.T) {
	logger := testutil.TestLogger()
	fileData := map[string]string{
		"app.star": `
def handler(dry_run, args):
    if dry_run:
	    return "Looks good"
    return ace.result(status="done", values=[{"a": {"c": 1}, "b": "abc"}], report=ace.AUTO)

app = ace.app("testApp",
	actions=[ace.action("testAction", "/", handler, show_validate=True)])

		`,
		"params.star": `param("param1", description="param1 description", type=STRING, default="myvalue")
param("param2", description="param2 description", type=BOOLEAN, default=False)`,
	}
	a, _, err := CreateTestApp(logger, fileData)
	if err != nil {
		t.Fatalf("Error %s", err)
	}

	request := httptest.NewRequest("GET", "/test/", nil)
	response := httptest.NewRecorder()
	a.ServeHTTP(response, request)

	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringContains(t, response.Body.String(), "/test/validate")
	if strings.Contains(response.Body.String(), "/test/suggest") {
		t.Errorf("suggest API should not be in the body")
	}

	request = httptest.NewRequest("POST", "/test/validate", nil)
	request.Header.Set("HX-Request", "true")
	response = httptest.NewRecorder()
	a.ServeHTTP(response, request)
	testutil.AssertEqualsInt(t, "code", 200, response.Code)
	testutil.AssertStringMatch(t, "body", `<div class="text-lg text-bold">
            Looks good
          </div>
        
          <div
            id="param_param1_error"
            hx-swap-oob="true"
            hx-swap="outerHTML"
            class="text-error mt-1">
            
          </div>
        
          <div
            id="param_param2_error"
            hx-swap-oob="true"
            hx-swap="outerHTML"
            class="text-error mt-1">
            
          </div>`, response.Body.String())
}
