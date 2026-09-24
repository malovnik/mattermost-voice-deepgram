package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// Fake embeds the real Plugin API, overriding only the contract used by this plugin.
// Unexpected API calls panic rather than silently succeeding.
type fakeAPI struct {
	plugin.API
	mu          sync.Mutex
	kv          map[string][]byte
	posts       map[string]*model.Post
	file        *model.FileInfo
	member      bool
	archived    bool
	reads       int
	creates     int
	createFails bool
	settings    map[string]string
}

func (f *fakeAPI) KVCompareAndSet(k string, old, new []byte) (bool, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !bytes.Equal(f.kv[k], old) {
		return false, nil
	}
	f.kv[k] = bytes.Clone(new)
	return true, nil
}
func (f *fakeAPI) KVCompareAndDelete(k string, old []byte) (bool, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !bytes.Equal(f.kv[k], old) {
		return false, nil
	}
	delete(f.kv, k)
	return true, nil
}
func (f *fakeAPI) KVGet(k string) ([]byte, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return bytes.Clone(f.kv[k]), nil
}
func (f *fakeAPI) GetPost(id string) (*model.Post, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p := f.posts[id]; p != nil {
		return p.Clone(), nil
	}
	return nil, model.NewAppError("test", "missing", nil, "", 404)
}
func (f *fakeAPI) GetChannelMember(c, u string) (*model.ChannelMember, *model.AppError) {
	if f.member {
		return &model.ChannelMember{}, nil
	}
	return nil, model.NewAppError("test", "forbidden", nil, "", 403)
}
func (f *fakeAPI) HasPermissionToChannel(string, string, *model.Permission) bool { return f.member }
func (f *fakeAPI) HasPermissionTo(string, *model.Permission) bool                { return false }
func (f *fakeAPI) GetChannel(id string) (*model.Channel, *model.AppError) {
	var at int64
	if f.archived {
		at = 1
	}
	return &model.Channel{Id: id, DeleteAt: at}, nil
}
func (f *fakeAPI) GetFileInfo(string) (*model.FileInfo, *model.AppError) { return f.file, nil }
func (f *fakeAPI) GetFile(string) ([]byte, *model.AppError) {
	f.reads++
	return []byte("audio-bytes"), nil
}
func (f *fakeAPI) GetPostThread(id string) (*model.PostList, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	list := model.NewPostList()
	for _, p := range f.posts {
		list.AddPost(p.Clone())
	}
	return list, nil
}
func (f *fakeAPI) CreatePost(p *model.Post) (*model.Post, *model.AppError) {
	if f.createFails {
		return nil, model.NewAppError("test", "rejected", nil, "", 400)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p = p.Clone()
	p.Id = model.NewId()
	f.posts[p.Id] = p
	f.creates++
	return p.Clone(), nil
}

func (f *fakeAPI) LogWarn(string, ...any) {}
func (f *fakeAPI) LoadPluginConfiguration(dest any) error {
	b, _ := json.Marshal(f.settings)
	return json.Unmarshal(b, dest)
}

func TestDeliveryFailureCannotChargeForever(t *testing.T) {
	p, f, post := fixture(t)
	f.createFails = true
	calls := 0
	p.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) { calls++; return response(200, goodResponse), nil })
	_ = p.enqueue(post.Id)
	for attempt := 1; attempt <= 6; attempt++ {
		p.process(jobPrefix + post.Id)
		if attempt < 6 {
			var j job
			_ = json.Unmarshal(f.kv[jobPrefix+post.Id], &j)
			j.Next = 0
			f.kv[jobPrefix+post.Id] = encodeJob(j)
		}
	}
	if calls != 3 || len(f.kv) != 0 {
		t.Fatalf("calls=%d jobs=%d", calls, len(f.kv))
	}
}
func TestBadAdminNumericSettingsDoNotBrickActivation(t *testing.T) {
	p, f, _ := fixture(t)
	f.settings = map[string]string{"MaxFileSizeMB": " 30 ", "MaxRecordingSeconds": "oops"}
	if err := p.OnConfigurationChange(); err != nil {
		t.Fatal(err)
	}
	if p.config().maxBytes() != 30*1024*1024 || p.config().seconds() != 300 {
		t.Fatal("missing safe defaults")
	}
}
func TestAttachmentReplicaLagCanRecover(t *testing.T) {
	p, f, post := fixture(t)
	f.file.PostId = ""
	p.MessageHasBeenPosted(nil, post)
	if len(f.kv) != 1 {
		t.Fatal("hook lost an unreplicated attachment")
	}
	p.process(jobPrefix + post.Id)
	if f.reads != 0 {
		t.Fatal("unattached file read")
	}
	var j job
	_ = json.Unmarshal(f.kv[jobPrefix+post.Id], &j)
	j.Next = 0
	f.kv[jobPrefix+post.Id] = encodeJob(j)
	f.file.PostId = post.Id
	p.process(jobPrefix + post.Id)
	if f.creates != 1 {
		t.Fatal("attachment did not recover")
	}
}
func (f *fakeAPI) UpdatePost(p *model.Post) (*model.Post, *model.AppError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posts[p.Id] = p.Clone()
	return p.Clone(), nil
}
func (f *fakeAPI) GetConfig() *model.Config {
	c := &model.Config{}
	c.SetDefaults()
	c.ServiceSettings.SiteURL = model.NewPointer("https://chat.example.com/teamchat")
	return c
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

const goodResponse = `{"results":{"channels":[{"alternatives":[{"transcript":"Привет, команда!"}]}]}}`

func fixture(t *testing.T) (*Plugin, *fakeAPI, *model.Post) {
	t.Helper()
	post := &model.Post{Id: model.NewId(), UserId: model.NewId(), ChannelId: model.NewId(), FileIds: []string{model.NewId()}}
	f := &fakeAPI{kv: map[string][]byte{}, posts: map[string]*model.Post{post.Id: post}, member: true, file: &model.FileInfo{Id: post.FileIds[0], PostId: post.Id, Name: "voice-message-1.webm", Size: 10, MimeType: "video/webm"}}
	c := defaultConfiguration()
	c.DeepgramAPIKey = "test-only-key"
	p := &Plugin{configuration: c, botID: model.NewId(), ctx: context.Background(), client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { return response(200, goodResponse), nil })}}
	p.SetAPI(f)
	return p, f, post
}

func TestHookQueueTranscriptionAndDedup(t *testing.T) {
	p, f, post := fixture(t)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); p.MessageHasBeenPosted(nil, post) }()
	}
	wg.Wait()
	if len(f.kv) != 1 {
		t.Fatalf("jobs=%d", len(f.kv))
	}
	p.process(jobPrefix + post.Id)
	if len(f.kv) != 0 || f.creates != 1 || f.reads != 1 {
		t.Fatalf("state jobs=%d creates=%d reads=%d", len(f.kv), f.creates, f.reads)
	}
	reply, err := p.existingReply(post)
	if err != nil || reply == nil || reply.RootId != post.Id || !strings.Contains(reply.Message, "Привет, команда") {
		t.Fatalf("reply=%+v, err=%v", reply, err)
	}
	_ = p.enqueue(post.Id)
	p.process(jobPrefix + post.Id)
	if f.creates != 1 || f.reads != 1 {
		t.Fatal("completed post was transcribed again")
	}
}
func TestReplyStaysInOriginalThread(t *testing.T) {
	p, f, post := fixture(t)
	post.RootId = model.NewId()
	_ = p.enqueue(post.Id)
	p.process(jobPrefix + post.Id)
	reply, _ := p.existingReply(post)
	if reply.RootId != post.RootId || reply.ChannelId != post.ChannelId || f.creates != 1 {
		t.Fatal("wrong destination")
	}
}
func TestLeaseExcludesConcurrentWorkersAndRestartRecovery(t *testing.T) {
	p, f, post := fixture(t)
	_ = p.enqueue(post.Id)
	f.kv[jobPrefix+post.Id] = encodeJob(job{PostID: post.Id, Next: time.Now().Add(time.Minute).Unix(), Lease: "other-node"})
	p.process(jobPrefix + post.Id)
	if f.reads != 0 {
		t.Fatal("stole live lease")
	}
	f.kv[jobPrefix+post.Id] = encodeJob(job{PostID: post.Id, Next: time.Now().Add(-time.Minute).Unix(), Lease: "crashed-node"})
	p.process(jobPrefix + post.Id)
	if f.creates != 1 {
		t.Fatal("did not recover expired lease")
	}
}
func TestTemporaryErrorRetriesThenUpdatesSingleFailureReply(t *testing.T) {
	p, f, post := fixture(t)
	calls := 0
	p.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) { calls++; return response(429, "DO NOT LOG SECRET"), nil })
	_ = p.enqueue(post.Id)
	for attempt := 1; attempt <= 3; attempt++ {
		p.process(jobPrefix + post.Id)
		if attempt < 3 {
			var j job
			_ = json.Unmarshal(f.kv[jobPrefix+post.Id], &j)
			if j.Attempts != attempt || j.Next <= time.Now().Unix() {
				t.Fatal("retry not scheduled")
			}
			j.Next = 0
			f.kv[jobPrefix+post.Id] = encodeJob(j)
		}
	}
	reply, _ := p.existingReply(post)
	if calls != 3 || f.creates != 1 || reply.GetProp("voice_status") != "failed" || strings.Contains(reply.Message, "SECRET") {
		t.Fatal("bad failure handling")
	}
	p.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return response(200, goodResponse), nil })
	_ = p.enqueue(post.Id)
	p.process(jobPrefix + post.Id)
	reply, _ = p.existingReply(post)
	if f.creates != 1 || reply.GetProp("voice_status") != "done" {
		t.Fatal("retry duplicated or failed to replace error")
	}
}
func TestDeletedArchivedOrDepartedSourceNeverSent(t *testing.T) {
	for _, condition := range []string{"deleted", "archived", "departed"} {
		t.Run(condition, func(t *testing.T) {
			p, f, post := fixture(t)
			switch condition {
			case "deleted":
				post.DeleteAt = 1
			case "archived":
				f.archived = true
			case "departed":
				f.member = false
			}
			_ = p.enqueue(post.Id)
			p.process(jobPrefix + post.Id)
			if f.reads != 0 || f.creates != 0 || len(f.kv) != 0 {
				t.Fatal("private/deleted audio processed")
			}
		})
	}
}
func TestSourceDeletedDuringProviderRequestSuppressesReply(t *testing.T) {
	p, f, post := fixture(t)
	p.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		post.DeleteAt = 1
		return response(200, goodResponse), nil
	})
	_ = p.enqueue(post.Id)
	p.process(jobPrefix + post.Id)
	if f.creates != 0 {
		t.Fatal("published after delete")
	}
}
func TestSizeAndAttachmentOwnershipChecked(t *testing.T) {
	p, f, post := fixture(t)
	f.file.Size = p.config().maxBytes() + 1
	p.MessageHasBeenPosted(nil, post)
	if len(f.kv) != 0 {
		t.Fatal("oversize queued")
	}
	f.file.Size = 10
	f.file.PostId = model.NewId()
	if _, e := p.firstAudio(post, p.config(), true); e == nil {
		t.Fatal("unrelated file accepted")
	}
}
func TestNoKeyDoesNotEnqueueOrExposeSecret(t *testing.T) {
	p, f, post := fixture(t)
	p.configuration.DeepgramAPIKey = ""
	p.MessageHasBeenPosted(nil, post)
	if len(f.kv) != 0 {
		t.Fatal("queued without key")
	}
	p.configuration.DeepgramAPIKey = "super-secret-value"
	r := httptest.NewRequest("GET", "/config", nil)
	r.Header.Set("Mattermost-User-ID", post.UserId)
	w := httptest.NewRecorder()
	p.ServeHTTP(nil, w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), "super-secret") || !strings.Contains(w.Body.String(), `"configured":true`) {
		t.Fatal(w.Body.String())
	}
}
func TestHTTPAuthorizationAndCSRF(t *testing.T) {
	p, _, post := fixture(t)
	for _, tc := range []struct {
		user, header string
		want         int
	}{{"", "", 401}, {post.UserId, "", 403}, {model.NewId(), "XMLHttpRequest", 400}, {post.UserId, "XMLHttpRequest", 202}} {
		r := httptest.NewRequest("POST", "/transcribe", strings.NewReader(`{"post_id":"`+post.Id+`"}`))
		r.Header.Set("Mattermost-User-ID", tc.user)
		r.Header.Set("X-Requested-With", tc.header)
		w := httptest.NewRecorder()
		p.ServeHTTP(nil, w, r)
		if w.Code != tc.want {
			t.Fatalf("got %d want %d: %s", w.Code, tc.want, w.Body.String())
		}
	}
}
func TestDeepgramWireContract(t *testing.T) {
	c := defaultConfiguration()
	c.DeepgramAPIKey = "test-only"
	c.Language = "auto"
	c.DeepgramRegion = "eu"
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		if r.URL.Host != "api.eu.deepgram.com" || r.URL.Query().Get("detect_language") != "true" || r.URL.Query().Has("language") || r.URL.Query().Get("mip_opt_out") != "true" || r.Header.Get("Authorization") != "Token test-only" || r.Header.Get("Content-Type") != "audio/webm" || string(b) != "audio" {
			t.Fatalf("bad wire contract: %s", r.URL)
		}
		return response(200, goodResponse), nil
	})}
	text, e := transcribe(context.Background(), client, c, []byte("audio"), "audio/webm")
	if e != nil || text != "Привет, команда!" {
		t.Fatalf("%q %v", text, e)
	}
}
func TestDeepgramFailureClasses(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		retry  bool
	}{{401, "secret", false}, {400, "secret", false}, {429, "secret", true}, {503, "secret", true}, {200, `{}`, true}, {200, strings.Repeat("x", 2*1024*1024+1), false}} {
		client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { return response(tc.status, tc.body), nil })}
		_, err := transcribe(context.Background(), client, defaultConfiguration(), []byte("a"), "audio/wav")
		te, ok := err.(*transcriptionError)
		if !ok || te.retryable != tc.retry || strings.Contains(te.Error(), "secret") {
			t.Fatalf("%d: %v", tc.status, err)
		}
	}
}
func TestTranscriptCannotPingOrInjectMarkdown(t *testing.T) {
	s := safeTranscript("@channel @here [click](https://evil.example) <script> *bold*")
	if strings.Contains(s, "@channel") || strings.Contains(s, "<script>") || strings.Contains(s, "[click]") {
		t.Fatal(s)
	}
	if len([]rune(safeTranscript(strings.Repeat("Я", 18000)))) > 14200 {
		t.Fatal("unbounded transcript")
	}
}
func TestConfigurationValidation(t *testing.T) {
	c := defaultConfiguration()
	if c.validate() != nil {
		t.Fatal("bad defaults")
	}
	c.MaxFileSizeMB = "999"
	if c.validate() == nil {
		t.Fatal("missing bound")
	}
	c = defaultConfiguration()
	c.DeepgramRegion = "http://localhost"
	if c.validate() == nil {
		t.Fatal("arbitrary endpoint accepted")
	}
}
