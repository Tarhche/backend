<template>
    <ckeditor :editor="ClassicEditor" :config="config" />
</template>
  
<script setup>
import CKEditor from '@ckeditor/ckeditor5-vue';
import ClassicEditor from '@jaroya/ckeditor5-build-full'

const config = {
	language: 'fa',
	toolbar: {
		items: [
			'undo',
			'redo',
			'|',
			'heading',
			'fontSize',
			'|',
			'removeFormat',
			'link',
			'bold',
			'italic',
			'underline',
			'|',
			'superscript',
			'subscript',
			'|',
			'bulletedList',
			'numberedList',
			'todolist',
			'|',
			'alignment',
			'outdent',
			'indent',
			'|',
			'insertTable',
			'blockQuote',
			'mediaEmbed',
			'|',
			'codeBlock',
			'code',
			'specialCharacters',
			'horizontalLine',
			'|',
			'sourceEditing',
		]
	},
	image: {
		toolbar: [
			'imageTextAlternative',
			'toggleImageCaption',
			'imageStyle:inline',
			'imageStyle:block',
			'imageStyle:side'
		],
	},
	table: {
		contentToolbar: [
			'tableColumn',
			'tableRow',
			'mergeTableCells',
			'tableCellProperties',
			'tableProperties'
		]
	},
	codeBlock: {
		languages: [
			{ language: 'plaintext', label: 'Plain text' },
			{ language: 'go', label: 'Golang' },
			{ language: 'c', label: 'C' },
			{ language: 'cs', label: 'C#' },
			{ language: 'cpp', label: 'C++' },
			{ language: 'css', label: 'CSS' },
			{ language: 'diff', label: 'Diff' },
			{ language: 'html', label: 'HTML' },
			{ language: 'java', label: 'Java' },
			{ language: 'javascript', label: 'JavaScript' },
			{ language: 'php', label: 'PHP' },
			{ language: 'python', label: 'Python' },
			{ language: 'ruby', label: 'Ruby' },
			{ language: 'typescript', label: 'TypeScript' },
			{ language: 'json', label: 'JSON' },
			{ language: 'xml', label: 'XML' }
		]
	}
}

const ckeditor = CKEditor.component
</script>
