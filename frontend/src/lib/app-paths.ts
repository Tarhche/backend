type Path = string | ((...args: any[]) => string);

export type PathSchema = Record<string, Path | Record<string, Path>>;

export const APP_PATHS = {
  home: "/",
  articles: {
    index: "/articles",
    detail: (uuid: string) => `/articles/${uuid}`,
  },
  auth: {
    login: "/auth/login",
    register: "/auth/register",
    verify: "/auth/verify",
    resetPassword: "/auth/reset-password",
    frogotPassword: "/auth/forgot-password",
  },
  hashtags: {
    index: "/hashtags",
  },
  dashboard: {
    index: "/dashboard",
    articles: "/dashboard/articles",
    articlesDetail: (uuid: string) => `/dashboard/articles/${uuid}`,
  },
} as const satisfies PathSchema;
