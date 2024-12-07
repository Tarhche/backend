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
    articles: {
      index: "/dashboard/articles",
      new: "/dashboard/articles/new",
      edit: (uuid: string) => `/dashboard/articles/${uuid}`,
    },
    comments: {
      index: "/dashboard/comments",
      edit: (uuid: string) => `/dashboard/comments/${uuid}`,
    },
    my: {
      comments: "/dashboard/my/comments",
      bookmarks: "/dashboard/my/bookmarks",
    },
    users: {
      index: "/dashboard/users",
      new: "/dashboard/users/new",
      edit: (uuid: string) => `/dashboard/users/${uuid}`,
      editPassword: (uuid: string) => `/dashboard/users/${uuid}/edit-password`,
    },
    roles: {
      index: "/dashboard/roles",
      new: "/dashboard/roles/new",
      edit: (uuid: string) => `/dashboard/roles/${uuid}`,
    },
    files: "/dashboard/files",
    settings: "/dashboard/settings",
    profile: {
      index: "/dashboard/profile",
      editPassword: "/dashboard/profile/password",
    },
  },
};
