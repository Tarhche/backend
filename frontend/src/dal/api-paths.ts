export const apiPaths = {
  home: "home",
  auth: {
    login: "auth/login",
    refreshToken: "auth/token/refresh",
    forgetPassword: "auth/password/forget",
    resetPassword: "auth/password/reset",
    register: "auth/register",
    verify: "auth/verify",
  },
  articles: {
    list: "articles",
    show: (uuid: string) => `articles/${uuid}`,
  },
  dashbaord: {
    profile: "dashboard/profile",
    roles: "dashboard/profile/roles",
    articles: "dashboard/articles",
    articlesDetail: (id: string) => `/dashboard/articles/${id}`,
    usersComments: "/dashboard/comments",
    usersCommentsDetail: (id: string) => `/dashboard/comments/${id}`,
  },
  comments: {
    list: "comments",
  },
  bookmarks: {
    bookmarks: "/bookmarks",
    exists: "/bookmarks/exists",
  },
  hashtags: {
    show: (slug: string) => `/hashtags/${slug}`,
  },
} as const;
