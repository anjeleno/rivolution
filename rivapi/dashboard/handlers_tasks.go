package dashboard

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/anjeleno/rivolution/rivapi/store"
)

type taskView struct {
	store.ScheduledTask
	LastRun store.TaskLastRun
}

type tasksPageData struct {
	baseData
	Tasks []taskView
	Error string
}

func (h *Handler) tasksPageData(r *http.Request) (tasksPageData, error) {
	tasks, err := store.LoadTasks(store.TasksConfigPath)
	if err != nil {
		return tasksPageData{}, err
	}
	data := tasksPageData{baseData: h.base(r, "Tasks", "tasks")}
	for _, t := range tasks {
		data.Tasks = append(data.Tasks, taskView{ScheduledTask: t, LastRun: store.TaskLastRunStatus(t.ID)})
	}
	if e := r.URL.Query().Get("error"); e != "" {
		data.Error = e
	}
	return data, nil
}

// Tasks handles GET /tasks.
func (h *Handler) Tasks(w http.ResponseWriter, r *http.Request) {
	data, err := h.tasksPageData(r)
	if err != nil {
		http.Error(w, "error loading tasks: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmplTasks.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// TasksAdd handles POST /tasks/add — creates and deploys a new task from
// the "Add task" form.
func (h *Handler) TasksAdd(w http.ResponseWriter, r *http.Request) {
	redirect := "/tasks"
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape("bad request"), http.StatusSeeOther)
		return
	}

	id, err := store.NewTaskID()
	if err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	t := store.ScheduledTask{
		ID:            id,
		Name:          strings.TrimSpace(r.FormValue("name")),
		Type:          store.TaskType(r.FormValue("type")),
		Enabled:       true,
		Schedule:      strings.TrimSpace(r.FormValue("schedule")),
		BackupDir:     strings.TrimSpace(r.FormValue("backup_dir")),
		RetentionDays: atoiDefault(r.FormValue("retention_days"), 14),
		FilePrefix:    strings.TrimSpace(r.FormValue("file_prefix")),
		ServiceName:   strings.TrimSpace(r.FormValue("service_name")),
		DaysOffset:    atoiDefault(r.FormValue("days_offset"), 1),
		CustomCommand: r.FormValue("custom_command"),
	}

	if t.Name == "" {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape("name is required"), http.StatusSeeOther)
		return
	}

	if err := store.DeployTask(t); err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape("deploying task: "+err.Error()), http.StatusSeeOther)
		return
	}

	tasks, err := store.LoadTasks(store.TasksConfigPath)
	if err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	tasks = append(tasks, t)
	if err := store.SaveTasks(tasks, store.TasksConfigPath); err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// TasksDelete handles POST /tasks/{id}/delete.
func (h *Handler) TasksDelete(w http.ResponseWriter, r *http.Request) {
	id := taskIDParam(r)
	redirect := "/tasks"

	if err := store.RemoveTask(id); err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	tasks, err := store.LoadTasks(store.TasksConfigPath)
	if err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	tasks = store.RemoveTaskFromList(tasks, id)
	if err := store.SaveTasks(tasks, store.TasksConfigPath); err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// TasksToggle handles POST /tasks/{id}/toggle — flips Enabled and
// redeploys (DeployTask itself enables/disables the timer to match).
func (h *Handler) TasksToggle(w http.ResponseWriter, r *http.Request) {
	id := taskIDParam(r)
	redirect := "/tasks"

	tasks, err := store.LoadTasks(store.TasksConfigPath)
	if err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	t, found := store.FindTask(tasks, id)
	if !found {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape("task not found"), http.StatusSeeOther)
		return
	}
	t.Enabled = !t.Enabled

	if err := store.DeployTask(t); err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	for i := range tasks {
		if tasks[i].ID == id {
			tasks[i] = t
		}
	}
	if err := store.SaveTasks(tasks, store.TasksConfigPath); err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// TasksRunNow handles POST /tasks/{id}/run — triggers the task's service
// unit immediately, out of band from its timer.
func (h *Handler) TasksRunNow(w http.ResponseWriter, r *http.Request) {
	id := taskIDParam(r)
	redirect := "/tasks"
	if err := store.RunTaskNow(id); err != nil {
		http.Redirect(w, r, redirect+"?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func taskIDParam(r *http.Request) string {
	return chi.URLParam(r, "id")
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}
