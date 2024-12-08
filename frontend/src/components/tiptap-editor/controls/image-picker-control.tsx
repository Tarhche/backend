import {useState} from "react";
import {Box, Paper, Modal} from "@mantine/core";
import {RichTextEditorControl, useRichTextEditorContext} from "@mantine/tiptap";
import {IconPhoto} from "@tabler/icons-react";
import {FilesExplorer} from "@/components/files-explorer";
import {FILES_PUBLIC_URL} from "@/constants";

export function ImagePickerControl() {
  const [isFileExplorerOpen, setIsFileExplorerOpen] = useState(false);
  const editor = useRichTextEditorContext();

  const handleOpenFileExplorer = () => {
    setIsFileExplorerOpen(true);
  };

  const handleSelectFile = (fileId: string | undefined) => {
    if (fileId) {
      editor.editor
        ?.chain()
        .focus()
        .setImage({src: `${FILES_PUBLIC_URL}/${fileId}`, alt: ""})
        .run();
      setIsFileExplorerOpen(false);
    }
  };

  const handleCloseFileExplorer = () => {
    setIsFileExplorerOpen(false);
  };

  return (
    <Box>
      <RichTextEditorControl
        aria-label="Import an image form files"
        title="وارد کردن عکس"
        onClick={handleOpenFileExplorer}
      >
        <IconPhoto size="1rem" stroke={1.5} />
      </RichTextEditorControl>
      <Modal
        size="xl"
        opened={isFileExplorerOpen}
        onClose={handleCloseFileExplorer}
        withCloseButton={false}
      >
        <Paper>
          <FilesExplorer onSelect={handleSelectFile} />
        </Paper>
      </Modal>
    </Box>
  );
}
