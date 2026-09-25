/* Backend speaking an incompatible ABI version. */
#include "../gosr.h"
#include <stdlib.h>

struct gosr_handle { int dummy; };

int gosr_abi_version(void) { return 999; }
gosr_handle *gosr_create(const char *a, int s, char *b, size_t l) {
    (void)a; (void)s; (void)b; (void)l;
    return (gosr_handle *)calloc(1, sizeof(gosr_handle));
}
void gosr_destroy(gosr_handle *h) { free(h); }
int gosr_load_model(gosr_handle *h, const char *p, char *b, size_t l) {
    (void)h; (void)p; (void)b; (void)l; return GOSR_OK;
}
int gosr_upsample(gosr_handle *h, const unsigned char *in, size_t in_len,
                  unsigned char **out, size_t *out_len,
                  char *errbuf, size_t errlen) {
    (void)h; (void)in; (void)in_len; (void)out; (void)out_len;
    (void)errbuf; (void)errlen; return GOSR_ERR_INTERNAL;
}
void gosr_free(void *p) { free(p); }
