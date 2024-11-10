import {notFound} from "next/navigation";
import {EditCommentForm} from "@/features/dashboard/components/edit-comment-form/edit-form";
import {fetchUsersDetailComments} from "@/dal";

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
      objectId={comment.object_uuid}
      message={comment.body}
      approvalDate={comment.approved_at}
    />
  );
}

export default ArticleDetalPage;
