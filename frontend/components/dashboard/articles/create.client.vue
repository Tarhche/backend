<script setup lang="ts">
import { ref } from 'vue';
import ClassicEditor from '@ckeditor/ckeditor5-build-classic';

// import { Essentials } from '@ckeditor/ckeditor5-essentials';
// import { Bold, Italic } from '@ckeditor/ckeditor5-basic-styles';
// import { Link } from '@ckeditor/ckeditor5-link';
// import { Paragraph } from '@ckeditor/ckeditor5-paragraph';
import "~/assets/css/ckEditorStyle.css"

// const editorConfig =ref( {
//   plugins: [
//     Essentials,
//     Bold,
//     Italic,
//     Link,
//     Paragraph
//   ],
//
//       toolbar: {
//     items: [
//       'bold',
//       'italic',
//       'link',
//       'undo',
//       'redo'
//     ]
//   }
// })




const title = ref("")
const tags = ref([])
const summary = ref("")
const editorData = ref("")

const image = ref()

const file = ref(null)

const editor = ref(ClassicEditor);

const getFile = ()=>{
   image.value = file.value.files[0]
}
const formData = ref({
  title:title.value,
  image:image.value,
  tags:tags.value,
  summary:summary.value,
  body:editorData.value
})
const sendArticle = async ()=>{
  const {data:response} = await useFetch('http://127.0.0.1:8000/dashboard/articles',{
    method:"post" ,
    body:{
      cover :formData.value.image,
      title:formData.value.title,
      body:formData.value.body,
      excerpt:formData.value.summary
    }
  })
  console.log(response.value)
}
</script>

<template>
  <div class="container">
    <div class="row">
      <div class="article" dir="rtl">
        <form action="" @submit.prevent="sendArticle">
          <div class="row">
            <div class=" col-md-6">
              <label class="form-label mt-2 " for="title">تیتر:</label>
              <input type="text" name="" id="title" class="form-control" v-model="title">
            </div>
            <div class="col-md-6">
              <label class="form-label mt-2" for="pic-news">تصویر خبر :</label>
              <input type="file" name="" id="pic-news" class="form-control" ref="file"  @change="getFile" accept="image/*">{{image}}
            </div>
            <div class="">
              <label class="form-label mt-2" for="subTitle">تگ ها :</label>
              <input type="text" name="" id="subTitle" class="form-control"  v-model="tags">
            </div>
            <div class="">
              <label class="form-label mt-2" for="summary">خلاصه توضیحات :</label>
              <textarea name="" id="summary" class="form-control" v-model="summary"></textarea>
            </div>
            <div id="container">
              <label class="form-label mt-2 " for="editor">توضیحات :</label>
              <ckeditor :editor="editor" v-model="editorData" class="text-start"  /> {{editorData}}
              <div class="input-group ">
                <button class="btn w-100 btn-primary mt-4">ارسال</button>
              </div>
            </div>
          </div>
        </form>
      </div>
    </div>
  </div>




</template>

<style scoped>
form label {
  cursor: pointer;
  font-weight: 600;
}
#summary{
height: 100px;
}
</style>