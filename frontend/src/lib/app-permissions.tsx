import {ExtractStrings} from "@/types/extractors";

type Action = "CREATE" | "DELETE" | "INDEX" | "SHOW" | "UPDATE";

type Permission = Partial<{
  [P in Action | (string & {})]: string | Permission;
}>;

type PermissionsSchema = {
  [P in string]: Permission;
};

export const PERMISSIONS = {
  articles: {
    CREATE: "articles.create",
    DELETE: "articles.delete",
    INDEX: "articles.index",
    SHOW: "articles.show",
    UPDATE: "articles.update",
  },
  comments: {
    CREATE: "comments.create",
    DELETE: "comments.delete",
    INDEX: "comments.index",
    SHOW: "comments.show",
    UPDATE: "comments.update",
  },
  config: {
    SHOW: "config.show",
    UPDATE: "config.update",
  },
  elements: {
    CREATE: "elements.create",
    DELETE: "elements.delete",
    INDEX: "elements.index",
    SHOW: "elements.show",
    UPDATE: "elements.update",
  },
  files: {
    CREATE: "files.create",
    DELETE: "files.delete",
    INDEX: "files.index",
    SHOW: "files.show",
  },
  permissions: {
    INDEX: "permissions.index",
  },
  roles: {
    CREATE: "roles.create",
    DELETE: "roles.delete",
    INDEX: "roles.index",
    SHOW: "roles.show",
    UPDATE: "roles.update",
  },
  self: {
    bookmarks: {
      DELETE: "self.bookmarks.delete",
      INDEX: "self.bookmarks.index",
    },
    comments: {
      DELETE: "self.comments.delete",
      INDEX: "self.comments.index",
      SHOW: "self.comments.show",
      UPDATE: "self.comments.update",
    },
  },
  users: {
    CREATE: "users.create",
    DELETE: "users.delete",
    INDEX: "users.index",
    SHOW: "users.show",
    UPDATE: "users.update",
    password: {
      UPDATE: "users.password.update",
    },
  },
} as const satisfies PermissionsSchema;

type PermissionsType = typeof PERMISSIONS;

export type Permissions = ExtractStrings<PermissionsType>;
