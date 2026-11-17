/* Test-only observation of the actual upstream RDP surface codec. */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdio.h>
#include <unistd.h>
#include <freerdp/gdi/gdi.h>
#include <freerdp/constants.h>
#include <freerdp/update.h>

static pSurfaceBits original_surface_bits;
static int last_codec = -1;

static BOOL observe_surface_bits(rdpContext* context, const SURFACE_BITS_COMMAND* command) {
    BOOL result = original_surface_bits(context, command);
    if (result && command->bmp.codecID != last_codec) {
        last_codec = command->bmp.codecID;
        const char* codec = command->bmp.codecID == RDP_CODEC_ID_REMOTEFX ? "remotefx"
            : command->bmp.codecID == RDP_CODEC_ID_NSCODEC ? "nscodec"
            : command->bmp.codecID == RDP_CODEC_ID_NONE ? "raw" : "unknown";
        fprintf(stderr, "RDP_CODEC pid=%ld codec=%s id=%u\n", (long)getpid(), codec, command->bmp.codecID);
    }
    return result;
}

BOOL gdi_init(freerdp* instance, UINT32 format) {
    BOOL (*real_init)(freerdp*, UINT32) = dlsym(RTLD_NEXT, "gdi_init");
    if (!real_init) {
        fprintf(stderr, "RDP_CODEC ERROR missing original gdi_init\n");
        return FALSE;
    }
    BOOL result = real_init(instance, format);
    if (result && instance->context->update->SurfaceBits) {
        original_surface_bits = instance->context->update->SurfaceBits;
        instance->context->update->SurfaceBits = observe_surface_bits;
        fprintf(stderr, "RDP_CODEC pid=%ld observer=surface\n", (long)getpid());
    }
    return result;
}
