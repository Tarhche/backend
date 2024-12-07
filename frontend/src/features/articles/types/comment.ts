export type Comment = {
  uuid: string;
  parent_uuid: string;
  body: string;
  created_at: string;
  author: {
    avatar: string;
    name: string;
  };
};
