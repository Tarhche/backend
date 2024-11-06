import {Stack, Group, Skeleton} from "@mantine/core";
import classes from "./comment.module.css";

export async function CommentsSkeleton() {
  return (
    <>
      <Stack mt={"xl"}>
        {[1, 2, 3].map((comment) => {
          return (
            <Group align="flex-start" mb={"md"} key={comment}>
              <Skeleton w={45} h={45} circle />
              <div className={classes.commentContent}>
                <Skeleton w={100} h={10} />
                <Skeleton my={"xs"} w={30} h={10} />
                <Skeleton w={"100%"} h={85} />
              </div>
            </Group>
          );
        })}
      </Stack>
    </>
  );
}
