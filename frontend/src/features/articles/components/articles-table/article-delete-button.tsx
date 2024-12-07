"use client";
import {useState} from "react";
import {
  Tooltip,
  Modal,
  ActionIcon,
  Button,
  Group,
  rem,
  Text,
} from "@mantine/core";
import {FormButton} from "@/components/form-button";
import {IconTrash} from "@tabler/icons-react";
import {deleteArticle} from "../../actions/delete-article";

type Props = {
  articleID: string;
  articleTitle?: string;
};

export function ArticleDeleteButton({articleID, articleTitle}: Props) {
  const [isConfirmOpen, setIsConfirmOpen] = useState(false);

  const handleSubmit = async () => {
    const fd = new FormData();
    fd.set("id", articleID);
    await deleteArticle(fd);
    setIsConfirmOpen(false);
  };

  return (
    <>
      <Tooltip label="حذف کردن مقاله" withArrow>
        <ActionIcon
          variant="light"
          size="lg"
          color="red"
          aria-label="حذف کردن مقاله"
          onClick={() => {
            setIsConfirmOpen(true);
          }}
        >
          <IconTrash style={{width: rem(20)}} stroke={1.5} />
        </ActionIcon>
      </Tooltip>
      <Modal
        title="تایید عملیات"
        opened={isConfirmOpen}
        size="md"
        centered
        onClose={() => {
          setIsConfirmOpen(false);
        }}
      >
        <Text>از حذف {`"${articleTitle}"`} مطمئن هستید؟</Text>
        <Group justify="flex-end" mt={"md"}>
          <Button
            color="gray"
            onClick={() => {
              setIsConfirmOpen(false);
            }}
          >
            لفو کردن
          </Button>
          <form action={handleSubmit}>
            <FormButton color="red">حذف کردن</FormButton>
          </form>
        </Group>
      </Modal>
    </>
  );
}
