import { defineRouteMiddleware } from "@astrojs/starlight/route-data";

export const onRequest = defineRouteMiddleware((context) => {
  context.locals.starlightRoute.entry.data.banner = {
    content:
      'crib is discontinued and no longer actively maintained. <a href="https://fabiorehm.com/blog/2026/10/09/im-done-with-dev-containers/">Read why</a>. Interested in taking over? <a href="https://github.com/fgrehm/crib/issues/95">Get in touch</a>.',
  };
});
