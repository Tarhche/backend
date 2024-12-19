import {useState} from "react";
import {Editor} from "reactjs-tiptap-editor";
import {
  ActionIcon,
  Modal,
  Stack,
  Group,
  TextInput,
  Button,
} from "@mantine/core";
import {FileInput} from "./file-input";
import {IconPhotoPlus} from "@tabler/icons-react";
import {FILES_PUBLIC_URL} from "@/constants";

type Props = {
  editor: Editor;
};

export function ImageUploadButton({editor}: Props) {
  const [isFileExplorerOpen, setIsFileExplorerOpen] = useState(false);

  const handleOpenFileExplorer = () => {
    setIsFileExplorerOpen(true);
  };

  const handleCloseFileExplorer = () => {
    setIsFileExplorerOpen(false);
  };

  const handleSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const src = fd.get("image")?.toString();
    const alt = fd.get("alt")?.toString();
    editor
      .chain()
      .focus()
      .setImage({
        src: `${FILES_PUBLIC_URL}/${src}`,
        alt: alt,
      })
      .run();
    setIsFileExplorerOpen(false);
  };

  return (
    <>
      <ActionIcon
        variant="transparent"
        color="white"
        size="xs"
        onClick={handleOpenFileExplorer}
      >
        <IconPhotoPlus />
      </ActionIcon>
      <Modal
        size="lg"
        title="وارد کردن تصویر"
        opened={isFileExplorerOpen}
        keepMounted={false}
        centered
        onClose={handleCloseFileExplorer}
      >
        <form onSubmit={handleSubmit}>
          <Stack gap="md">
            <Stack gap="md">
              <FileInput />
              <TextInput
                name="alt"
                label="متن جایگزین"
                placeholder="alt text"
              />
            </Stack>
            <Group justify="end">
              <Button type="submit">تایید</Button>
            </Group>
          </Stack>
        </form>
      </Modal>
    </>
  );
}
