/* Backend deliberately missing the mandatory gosr_upsample symbol, to
 * exercise the "library function missing" error path. */
#include "../gosr.h"
#include <stdlib.h>

struct gosr_handle { int dummy; };

int gosr_abi_version(void) { return GOSR_ABI_VERSION; }
gosr_handle *gosr_create(const char *a, int s, char *b, size_t l) {
    (void)a; (void)s; (void)b; (void)l;
    return (gosr_handle *)calloc(1, sizeof(gosr_handle));
}
void gosr_destroy(gosr_handle *h) { free(h); }
int gosr_load_model(gosr_handle *h, const char *p, char *b, size_t l) {
    (void)h; (void)p; (void)b; (void)l; return GOSR_OK;
}
/* gosr_upsample intentionally absent. */
void gosr_free(void *p) { free(p); }
