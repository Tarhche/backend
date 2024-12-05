import {notFound} from "next/navigation";
import {EditCommentForm} from "@/features/comments/components/edit-comment-form";
import {fetchUsersDetailComments} from "@/dal/private/comments";
import {withPermissions} from "@/components/with-authorization";

export const metadata = {
  title: "ویرایش کامنت",
};

type Props = {
  params: {
    uuid?: string;
  };
};

async function ArticleDetalPage({params}: Props) {
  const {uuid} = params;

  if (uuid === undefined) {
    notFound();
  }

  const comment = await fetchUsersDetailComments(uuid);

  return (
    <EditCommentForm
      id={comment.uuid}
      parentId={comment.parent_uuid}
      objectId={comment.object_uuid}
      message={comment.body}
      approvalDate={comment.approved_at}
    />
  );
}

export default withPermissions(ArticleDetalPage, {
  requiredPermissions: ["comments.show", "comments.update"],
  operator: "AND",
});
