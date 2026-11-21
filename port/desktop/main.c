/*
 * LaborControl R11 experimental native desktop shell for ALT Linux.
 * GTK3 + WebKit2GTK 4.1 render the existing local R11 interface in a native
 * window; the unchanged Go backend remains a separate, locally bound child.
 * No Chromium, internet access or remote page is required.
 */
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <gio/gio.h>
#include <glib/gstdio.h>
#include <string.h>

#define LOOPBACK_HOST "127.0.0.1"
#define LOOPBACK_PORT 8779
#define APP_URL "http://127.0.0.1:8779/r11"
#define APP_ORIGIN "http://127.0.0.1:8779/"

typedef struct {
    GtkWidget *window;
    WebKitWebView *web;
    GSubprocess *backend;
    gchar *backend_path;
    gchar *install_dir;
    gchar *instance_token;
    guint checks;
    gboolean page_opened;
    gboolean closing;
    gboolean backend_finished;
} Desktop;

static Desktop desktop = {0};

static GSocketConnection *connect_loopback(void) {
    GSocketClient *client = g_socket_client_new();
    g_socket_client_set_timeout(client, 1);
    GError *error = NULL;
    GSocketConnection *conn = g_socket_client_connect_to_host(
        client, LOOPBACK_HOST, LOOPBACK_PORT, NULL, &error);
    g_clear_error(&error);
    g_object_unref(client);
    return conn;
}

static gboolean server_port_occupied(void) {
    GSocketConnection *conn = connect_loopback();
    if (conn == NULL) return FALSE;
    g_object_unref(conn);
    return TRUE;
}

static gchar *local_http_request(const gchar *request) {
    GSocketConnection *conn = connect_loopback();
    if (conn == NULL) return NULL;
    GError *error = NULL;
    GOutputStream *output = g_io_stream_get_output_stream(G_IO_STREAM(conn));
    gboolean written = g_output_stream_write_all(
        output, request, strlen(request), NULL, NULL, &error);
    gchar *result = NULL;
    if (written) {
        GInputStream *input = g_io_stream_get_input_stream(G_IO_STREAM(conn));
        GString *buffer = g_string_sized_new(1024);
        gchar chunk[2048];
        gssize count;
        while (buffer->len < 8192 &&
               (count = g_input_stream_read(input, chunk, sizeof(chunk), NULL, &error)) > 0) {
            g_string_append_len(buffer, chunk, count);
        }
        result = g_string_free(buffer, FALSE);
    }
    g_clear_error(&error);
    g_io_stream_close(G_IO_STREAM(conn), NULL, NULL);
    g_object_unref(conn);
    return result;
}

static void show_error(const gchar *message) {
    GtkWidget *dialog = gtk_message_dialog_new(
        desktop.window != NULL ? GTK_WINDOW(desktop.window) : NULL,
        GTK_DIALOG_MODAL, GTK_MESSAGE_ERROR, GTK_BUTTONS_CLOSE,
        "%s", message);
    gtk_window_set_title(GTK_WINDOW(dialog), "LaborControl R11");
    gtk_dialog_run(GTK_DIALOG(dialog));
    gtk_widget_destroy(dialog);
}

static void request_backend_shutdown(void) {
    /* The backend responds, waits 250 ms and terminates itself. */
    gchar *response = local_http_request(
        "POST /api/shutdown HTTP/1.0\r\n"
        "Host: 127.0.0.1\r\n"
        "Content-Length: 0\r\nConnection: close\r\n\r\n");
    g_free(response);
}

static void on_window_destroy(GtkWidget *widget, gpointer user_data) {
    (void)widget;
    (void)user_data;
    desktop.closing = TRUE;
    if (desktop.backend != NULL && !desktop.backend_finished) {
        request_backend_shutdown();
    }
}

static void on_backend_exit(GObject *object, GAsyncResult *result, gpointer user_data) {
    (void)user_data;
    GError *error = NULL;
    if (!g_subprocess_wait_finish(G_SUBPROCESS(object), result, &error)) {
        g_warning("Backend wait: %s", error != NULL ? error->message : "unknown");
    }
    g_clear_error(&error);
    desktop.backend_finished = TRUE;
    if (!desktop.closing && desktop.window != NULL) {
        /* A normal /api/shutdown request from R11 is not an application crash. */
        if (!g_subprocess_get_if_exited(desktop.backend) ||
            g_subprocess_get_exit_status(desktop.backend) != 0) {
            show_error("Локальный сервер LaborControl завершился с ошибкой. Проверьте журнал приложения.");
        }
        gtk_widget_destroy(desktop.window);
    }
}

/* Compare parsed scheme/host/port, not a string prefix. A URL such as
 * http://127.0.0.1:8779@untrusted.example must never load inside our window. */
static gboolean app_local_uri_allowed(const gchar *uri) {
    if (uri == NULL) return FALSE;
    GError *error = NULL;
    GUri *parsed = g_uri_parse(uri, G_URI_FLAGS_NONE, &error);
    g_clear_error(&error);
    if (parsed == NULL) return FALSE;
    gboolean allowed =
        g_strcmp0(g_uri_get_scheme(parsed), "http") == 0 &&
        g_strcmp0(g_uri_get_host(parsed), LOOPBACK_HOST) == 0 &&
        g_uri_get_port(parsed) == LOOPBACK_PORT &&
        g_uri_get_userinfo(parsed) == NULL;
    g_uri_unref(parsed);
    return allowed;
}

static gboolean on_decide_navigation(WebKitWebView *web, WebKitPolicyDecision *decision,
                                      WebKitPolicyDecisionType kind, gpointer user_data) {
    (void)web;
    (void)user_data;
    if (kind != WEBKIT_POLICY_DECISION_TYPE_NAVIGATION_ACTION) return FALSE;
    if (!WEBKIT_IS_NAVIGATION_POLICY_DECISION(decision)) return FALSE;
    WebKitURIRequest *request = webkit_navigation_policy_decision_get_request(
        WEBKIT_NAVIGATION_POLICY_DECISION(decision));
    const gchar *uri = webkit_uri_request_get_uri(request);
    if (uri != NULL &&
        (app_local_uri_allowed(uri) || g_str_has_prefix(uri, "about:blank"))) {
        return FALSE;
    }
    webkit_policy_decision_ignore(decision);
    /* External destinations must not replace our local application. */
    if (uri != NULL && (g_str_has_prefix(uri, "https://") || g_str_has_prefix(uri, "http://"))) {
        gtk_show_uri_on_window(GTK_WINDOW(desktop.window), uri, GDK_CURRENT_TIME, NULL);
    }
    return TRUE;
}

static gboolean on_choose_download_destination(WebKitDownload *download,
                                                gchar *suggested_filename,
                                                gpointer user_data) {
    (void)user_data;
    GtkWidget *dialog = gtk_file_chooser_dialog_new(
        "Сохранить файл LaborControl", GTK_WINDOW(desktop.window),
        GTK_FILE_CHOOSER_ACTION_SAVE,
        "_Отмена", GTK_RESPONSE_CANCEL,
        "_Сохранить", GTK_RESPONSE_ACCEPT, NULL);
    gtk_file_chooser_set_do_overwrite_confirmation(GTK_FILE_CHOOSER(dialog), TRUE);
    gtk_file_chooser_set_current_name(GTK_FILE_CHOOSER(dialog),
        suggested_filename != NULL ? suggested_filename : "laborcontrol-export");
    if (gtk_dialog_run(GTK_DIALOG(dialog)) == GTK_RESPONSE_ACCEPT) {
        gchar *path = gtk_file_chooser_get_filename(GTK_FILE_CHOOSER(dialog));
        gchar *uri = g_filename_to_uri(path, NULL, NULL);
        if (uri != NULL) webkit_download_set_destination(download, uri);
        g_free(path);
        g_free(uri);
    } else {
        webkit_download_cancel(download);
    }
    gtk_widget_destroy(dialog);
    return TRUE;
}

static void on_download_started(WebKitWebContext *context, WebKitDownload *download,
                                gpointer user_data) {
    (void)context;
    (void)user_data;
    g_signal_connect(download, "decide-destination",
                     G_CALLBACK(on_choose_download_destination), NULL);
}

static gboolean try_open_page(gpointer user_data) {
    (void)user_data;
    if (desktop.closing || desktop.page_opened) return G_SOURCE_REMOVE;
    gchar *response = local_http_request(
        "GET /health HTTP/1.0\r\nHost: 127.0.0.1\r\nConnection: close\r\n\r\n");
    gboolean own_server =
        response != NULL &&
        strstr(response, "R11-LINUX-CORE/0.1") != NULL &&
        desktop.instance_token != NULL &&
        strstr(response, desktop.instance_token) != NULL;
    g_free(response);
    if (own_server) {
        desktop.page_opened = TRUE;
        webkit_web_view_load_uri(desktop.web, APP_URL);
        return G_SOURCE_REMOVE;
    }
    desktop.checks++;
    if (desktop.checks >= 80) {
        show_error("Не удалось запустить локальный сервер R11. Проверьте журнал и порт 8779.");
        gtk_widget_destroy(desktop.window);
        return G_SOURCE_REMOVE;
    }
    return G_SOURCE_CONTINUE;
}

static gchar *resolved_install_dir(void) {
    GError *error = NULL;
    gchar *self_path = g_file_read_link("/proc/self/exe", &error);
    if (self_path == NULL) {
        g_clear_error(&error);
        return NULL;
    }
    gchar *dir = g_path_get_dirname(self_path);
    g_free(self_path);
    return dir;
}

static void on_activate(GtkApplication *application, gpointer user_data) {
    (void)user_data;
    if (desktop.window != NULL) {
        gtk_window_present(GTK_WINDOW(desktop.window));
        return;
    }

    desktop.install_dir = resolved_install_dir();
    if (desktop.install_dir == NULL) {
        show_error("Не удалось определить каталог установленного приложения.");
        return;
    }
    desktop.backend_path = g_build_filename(desktop.install_dir, "laborcontrol-controller", NULL);
    if (!g_file_test(desktop.backend_path, G_FILE_TEST_IS_EXECUTABLE)) {
        show_error("Рядом с laborcontrol-desktop отсутствует исполняемый laborcontrol-controller.");
        return;
    }
    if (server_port_occupied()) {
        show_error("Порт 8779 уже занят. Закройте прежний LaborControl перед запуском настольной версии.");
        return;
    }

    desktop.window = gtk_application_window_new(application);
    gtk_window_set_title(GTK_WINDOW(desktop.window), "LaborControl R11 — экспериментальная настольная версия");
    gtk_window_set_default_size(GTK_WINDOW(desktop.window), 1380, 840);
    gtk_window_set_position(GTK_WINDOW(desktop.window), GTK_WIN_POS_CENTER);
    gchar *icon_file = g_build_filename(desktop.install_dir, "laborcontrol.svg", NULL);
    gtk_window_set_icon_from_file(GTK_WINDOW(desktop.window), icon_file, NULL);
    g_free(icon_file);
    g_signal_connect(desktop.window, "destroy", G_CALLBACK(on_window_destroy), NULL);

    desktop.web = WEBKIT_WEB_VIEW(webkit_web_view_new());
    webkit_web_view_set_settings(desktop.web, webkit_settings_new());
    g_object_set(webkit_web_view_get_settings(desktop.web),
                 "enable-javascript", TRUE, NULL);
    g_signal_connect(desktop.web, "decide-policy", G_CALLBACK(on_decide_navigation), NULL);
    WebKitWebContext *context = webkit_web_view_get_context(desktop.web);
    g_signal_connect(context, "download-started", G_CALLBACK(on_download_started), NULL);
    gtk_container_add(GTK_CONTAINER(desktop.window), GTK_WIDGET(desktop.web));
    gtk_widget_show_all(desktop.window);

    /* Dedicated profile by default, deliberately separate from old previews and production. */
    const gchar *override_home = g_getenv("LABORCONTROL_DESKTOP_DATA_HOME");
    gchar *sandbox = NULL;
    if (override_home != NULL && *override_home != '\0') {
        if (!g_path_is_absolute(override_home)) {
            show_error("LABORCONTROL_DESKTOP_DATA_HOME должен быть абсолютным путём.");
            gtk_widget_destroy(desktop.window);
            return;
        }
        sandbox = g_strdup(override_home);
    } else {
        sandbox = g_build_filename(g_get_user_data_dir(),
                    "LaborControl-R11-Desktop-Experimental", NULL);
    }
    if (g_mkdir_with_parents(sandbox, 0700) != 0) {
        show_error("Не удалось создать отдельный каталог тестовых данных.");
        gtk_widget_destroy(desktop.window);
        g_free(sandbox);
        return;
    }

    desktop.instance_token = g_uuid_string_random();
    GSubprocessLauncher *launcher = g_subprocess_launcher_new(G_SUBPROCESS_FLAGS_NONE);
    g_subprocess_launcher_setenv(launcher, "XDG_DATA_HOME", sandbox, TRUE);
    g_subprocess_launcher_setenv(launcher, "LABORCONTROL_DESKTOP_MODE", "1", TRUE);
    g_subprocess_launcher_setenv(launcher, "LABORCONTROL_DESKTOP_TOKEN",
                                 desktop.instance_token, TRUE);
    GError *error = NULL;
    desktop.backend = g_subprocess_launcher_spawn(
        launcher, &error, desktop.backend_path, NULL);
    g_object_unref(launcher);
    g_free(sandbox);
    if (desktop.backend == NULL) {
        show_error(error != NULL ? error->message : "Не удалось запустить Go Core.");
        g_clear_error(&error);
        gtk_widget_destroy(desktop.window);
        return;
    }
    g_subprocess_wait_async(desktop.backend, NULL, on_backend_exit, NULL);
    g_timeout_add(250, try_open_page, NULL);
}

int main(int argc, char **argv) {
    if (argc == 2 && strcmp(argv[1], "--check") == 0) {
        gchar *dir = resolved_install_dir();
        gchar *backend = dir != NULL ? g_build_filename(dir, "laborcontrol-controller", NULL) : NULL;
        gboolean ok = backend != NULL && g_file_test(backend, G_FILE_TEST_IS_EXECUTABLE);
        /* Also smoke-test the navigation origin guard, without a display. */
        ok = ok && app_local_uri_allowed(APP_URL) &&
             app_local_uri_allowed("http://127.0.0.1:8779/tab2?embed=tab2") &&
             !app_local_uri_allowed("http://127.0.0.1:8779@untrusted.example/") &&
             !app_local_uri_allowed("http://untrusted.example/") &&
             !app_local_uri_allowed("https://127.0.0.1:8779/r11");
        g_print("LaborControl R11 desktop shell: GTK %d.%d / WebKitGTK %u.%u; backend %s\n",
                gtk_get_major_version(), gtk_get_minor_version(),
                webkit_get_major_version(), webkit_get_minor_version(),
                ok ? "present" : "missing");
        g_free(backend);
        g_free(dir);
        return ok ? 0 : 2;
    }
    if (argc == 2 && strcmp(argv[1], "--version") == 0) {
        g_print("LaborControl-R11-Desktop-Experimental/0.1\n");
        return 0;
    }
    GtkApplication *application = gtk_application_new(
        "org.laborcontrol.R11Experimental", G_APPLICATION_FLAGS_NONE);
    g_signal_connect(application, "activate", G_CALLBACK(on_activate), NULL);
    int status = g_application_run(G_APPLICATION(application), argc, argv);
    g_object_unref(application);
    if (desktop.backend != NULL) g_object_unref(desktop.backend);
    g_free(desktop.backend_path);
    g_free(desktop.install_dir);
    g_free(desktop.instance_token);
    return status;
}
