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
    newArticle: "/dashboard/articles/new",
    comments: "/dashboard/comments",
    editComment: (uuid: string) => `/dashboard/comments/edit/${uuid}`,
    myComments: "/dashboard/my/comments",
    myBookmarks: "/dashboard/my/bookmarks",
    users: "/dashboard/users",
    newUser: "/dashboard/users/new",
    editUser: (uuid: string) => `/dashboard/users/${uuid}`,
    roles: "/dashboard/roles",
    newRole: "/dashboard/roles/new",
    editRole: (uuid: string) => `/dashboard/roles/${uuid}`,
    files: "/dashboard/files",
    settings: "/dashboard/settings",
    profile: "/dashboard/profile",
  },
} as const satisfies PathSchema;
