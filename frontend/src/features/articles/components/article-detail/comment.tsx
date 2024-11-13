"use client";
import {useState} from "react";
import {
  Text,
  Avatar,
  Group,
  Box,
  Paper,
  Button,
  Tooltip,
  Skeleton,
} from "@mantine/core";
import {CommentForm} from "./comment-form";
import {useInit} from "@/hooks/data/init";
import {IconCornerUpLeft, IconX} from "@tabler/icons-react";
import clsx from "clsx";
import {dateFromNow} from "@/lib/date-and-time";
import {type Comment as CommentType} from "../../types/comment";
import {FILES_PUBLIC_URL} from "@/constants/envs";
import classes from "./comment.module.css";

type Props = {
  // This objectUUID is related to the article that the comment will be linked to
  objectUUID: string;
  comment: CommentType;
  comments: CommentType[];
  level?: number;
};

export function Comment({objectUUID, comment, comments, level = 0}: Props) {
  const {data, isLoading} = useInit();
  const isLoggedIn = data?.status === "authenticated";
  const [isReplying, setIsReplying] = useState(false);
  const {uuid, author, body, created_at} = comment;
  const {name, avatar} = author;
  const replies = comments.filter((c) => c.parent_uuid === uuid);

  return (
    <Paper
      mb="xs"
      className={clsx({
        [classes.comment]: true,
        [classes.rootComment]: level === 0,
        [classes.nestedComment]: level > 0,
      })}
      pb={isLoggedIn ? 0 : "sm"}
    >
      <Group align="flex-start">
        <Avatar src={`${FILES_PUBLIC_URL}/${avatar}`} radius="xl" />
        <div className={classes.commentContent}>
          <Text size="sm" fw={500}>
            {name}
          </Text>
          <Text size="xs" c="dimmed">
            {dateFromNow(created_at)}
          </Text>
          <Text mt="xs">{body}</Text>
          {isLoading ? (
            <Skeleton w={30} h={25} className={classes.replyButton} />
          ) : isLoggedIn ? (
            <Tooltip label={"پاسخ دادن"} withArrow>
              <Button
                className={classes.replyButton}
                variant="transparent"
                c="dimmed"
                size="xs"
                mt="xs"
                onClick={() => {
                  setIsReplying(!isReplying);
                }}
              >
                {isReplying ? (
                  <IconX size={25} />
                ) : (
                  <IconCornerUpLeft size={25} />
                )}
              </Button>
            </Tooltip>
          ) : null}
        </div>
      </Group>
      {isReplying && (
        <Box mt={"xs"}>
          <CommentForm objectUUID={objectUUID} parentUUID={uuid ?? null} />
        </Box>
      )}
      {replies && (
        <div style={{marginTop: 10}}>
          {replies.map((reply, index) => (
            <Comment
              key={index}
              objectUUID={objectUUID}
              comment={reply}
              comments={comments}
              level={level + 1}
            />
          ))}
        </div>
      )}
    </Paper>
  );
}
