import Link from "next/link";
import {
  Box,
  Stack,
  Group,
  Button,
  Textarea,
  Title,
  Alert,
  Anchor,
} from "@mantine/core";
import {AuthGuard} from "@/components/auth-guard";
import {Comment} from "./comment";
import {IconMessage, IconInfoCircle} from "@tabler/icons-react";
import {fetchArticleComments} from "@/dal/comments";
import {FILES_PUBLIC_URL} from "@/constants/envs";
import {dateFromNow} from "@/lib/date-and-time";

type Props = {
  uuid: string;
};

export async function Comments({uuid}: Props) {
  const comments = (await fetchArticleComments(uuid)).items;

  return (
    <Box>
      <Group align="center" mb={"sm"} gap={"sm"}>
        <IconMessage />
        <Title ta={"right"} order={3}>
          دیدگاه ها
        </Title>
      </Group>
      <AuthGuard
        fallback={
          <Alert
            variant="light"
            color="yellow"
            title="نیازمند احراز هویت"
            icon={<IconInfoCircle />}
          >
            برای اینکه بتوانید دیدگاه خود را ثبت کنید باید ابتدا{" "}
            <Anchor underline="always" href={"/auth/login"} component={Link}>
              وارد حسابتان
            </Anchor>{" "}
            شوید
          </Alert>
        }
      >
        <Stack align="flex-end">
          <Textarea
            placeholder="دیدگاه خود را اینجا بنویسید"
            w={"100%"}
            rows={4}
          />
          <Button>ثبت نظر</Button>
        </Stack>
      </AuthGuard>
      <Stack mt={"lg"}>
        {comments.map((comment) => {
          return (
            <Comment
              key={comment.uuid}
              avatar={`${FILES_PUBLIC_URL}/${comment.author.avatar}`}
              name={comment.author.name}
              message={comment.body}
              date={dateFromNow(comment.created_at)}
            />
          );
        })}
        {comments.length === 0 && (
          <Alert variant="light" color="green" icon={<IconInfoCircle />}>
            هنوز دیدگاهی برای ان مقاله ثبت نشده!
          </Alert>
        )}
      </Stack>
    </Box>
  );
}
