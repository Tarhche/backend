import {NotFound} from "@/components/not-found";

function NotFoundPage() {
  return (
    <NotFound
      anchorText="داشبرد"
      title="این صفحه وجود ندارد"
      anchorLink="/dashboard"
    />
  );
}

export default NotFoundPage;
