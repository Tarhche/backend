import {Box, Stack, Group, Button, Textarea, Title} from "@mantine/core";
import {Comment} from "./comment";
import {IconMessage} from "@tabler/icons-react";
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
      <Stack align="flex-end">
        <Textarea
          placeholder="دیدگاه خود را اینجا بنویسید"
          w={"100%"}
          rows={4}
        />
        <Button>ثبت نظر</Button>
      </Stack>
      <Stack>
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
      </Stack>
    </Box>
  );
}
