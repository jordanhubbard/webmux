/* Test-only ordering control for the legacy guacd user-registration race. */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <pthread.h>
#include <unistd.h>
#include <fcntl.h>
#include <errno.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <time.h>

static pthread_mutex_t lock = PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t changed = PTHREAD_COND_INITIALIZER;
static int size_seen;
struct start { void* (*fn)(void*); void* data; };

static int wait_for(int* value) {
    struct timespec until;
    clock_gettime(CLOCK_REALTIME, &until);
    until.tv_sec += 2;
    int result = 0;
    while (!*value && !result) result = pthread_cond_timedwait(&changed, &lock, &until);
    return *value;
}
/* Give the baseline connection a head start toward registration. Its actual
 * blue/green pixels are still required; this delay is not admission evidence. */
static void* baseline_start(void* opaque) {
    struct start start = *(struct start*)opaque;
    free(opaque);
    usleep(100000);
    return start.fn(start.data);
}
int pthread_create(pthread_t* thread, const pthread_attr_t* attr, void* (*fn)(void*), void* data) {
    int (*real_create)(pthread_t*,const pthread_attr_t*,void*(*)(void*),void*) = dlsym(RTLD_NEXT,"pthread_create");
    Dl_info symbol;
    if (!getenv("WEBMUX_RDP_SCHEDULE_MARKER") || !dladdr((void*)fn,&symbol) || !symbol.dli_sname || strcmp(symbol.dli_sname,"guac_rdp_client_thread"))
        return real_create(thread,attr,fn,data);
    const char* marker = getenv("WEBMUX_RDP_SCHEDULE_MARKER");
    int fd = open(marker, O_WRONLY|O_CREAT|O_EXCL, 0600);
    int force_race = fd < 0 && errno == EEXIST;
    if (fd >= 0) close(fd);
    else if (!force_race) fprintf(stderr, "RDP_SCHEDULE ERROR marker creation failed\n");
    if (!force_race) {
        struct start* start = malloc(sizeof(*start));
        if (!start) return ENOMEM;
        *start = (struct start){fn,data};
        fprintf(stderr,"RDP_SCHEDULE baseline client starts after registration head start\n");
        int result = real_create(thread,attr,baseline_start,start);
        if (result) free(start);
        return result;
    }
    pthread_mutex_lock(&lock);
    size_seen = 0;
    pthread_mutex_unlock(&lock);
    int result = real_create(thread,attr,fn,data);
    if (!result) {
        pthread_mutex_lock(&lock);
        int observed = wait_for(&size_seen);
        pthread_mutex_unlock(&lock);
        fprintf(stderr,"RDP_SCHEDULE delayed registration; initial_size_before_registration=%d\n",observed);
    }
    return result;
}
int guac_protocol_send_size(void* socket, const void* layer, int width, int height) {
    int (*real_size)(void*,const void*,int,int) = dlsym(RTLD_NEXT,"guac_protocol_send_size");
    int result = real_size(socket,layer,width,height);
    if (width == 1024 && height == 768) {
        pthread_mutex_lock(&lock);
        size_seen = 1; pthread_cond_broadcast(&changed);
        pthread_mutex_unlock(&lock);
    }
    return result;
}
