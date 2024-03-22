<script setup lang="ts">
import {ref} from 'vue';
import ClassicEditor from '@ckeditor/ckeditor5-build-classic';
import {useTarcheApi} from "~/store/tarche";
import "~/assets/css/ckEditorStyle.css"
const {public:{baseURL}} = useRuntimeConfig()
const emit = defineEmits(["send"])
const store = useTarcheApi()
const tagsElement = ref(null)
const props = defineProps(['data'])
const tags = ref("")

const fileData = reactive({
  cover: [],
  title: "",
  tags: [],
  excerpt: "",
  body: "",

})





const file = ref(null)
let form = new FormData()

const editor = ref(ClassicEditor);

const getFile = async () => {
  console.log(file.value.files[0])
  form.append("file" ,file.value.files[0] )
}
async function sendFile(){
  console.log()
  const cookie = useCookie("tarche")
  const {data:data, error} = await useFetch(`${baseURL}/api/dashboard/files`,{
    method:"POST" ,
    headers:{
      Authorization: `Bearer ${cookie.value}`,
    },
    body:form
  })
  console.log(error , "error")
  fileData.cover =JSON.parse(data.value).uuid
}
function pushTags() {
  tags.value = tags.value.trim()
  if (!fileData.tags.includes(tags.value) && tags.value.length ) {
    fileData.tags.push(tags.value)
    tags.value = ""
    tagsElement.value.focus()
  }
  else {
    tags.value = ""
    tagsElement.value.focus()
  }
}

function deleteTags(index) {
  fileData.tags.splice(index, 1)
}

function sendArticle() {

  if (fileData.tags.length && fileData.title && fileData.body && fileData.excerpt && fileData.cover.length) {
    emit("send", fileData)
  }
  else {
    console.log(fileData)
  }
}

/* get data for edite and show post */

if (props.data) {
  console.log(props.data)
  const {data:data, error} = await useFetch(() => `https://tarhche-backend.liara.run/api/articles/${props.data}`)
if (error.value){
  console.log( "error" , error.value)

}
if (data.value){
  fileData.title = data.value.title
  fileData.tags = data.value.tags
  fileData.excerpt = data.value.excerpt
  fileData.body = data.value.body
  fileData.cover = data.value.cover
}
}

</script>

<template>
  <div class="container">
    <div class="row">
      <div class="article" dir="rtl">
        <form action="" @submit.prevent="sendArticle">
          <div class="row">
            <div class="  ">
              <label class="form-label mt-2 " for="title">تیتر:</label>
              <input type="text" name="" id="title" class=" form-control shadow-sm rounded-1" v-model="fileData.title">
            </div>
            <div class="d-flex flex-column  ">
              <label class="form-label mt-2" for="pic-news">تصویر خبر :</label>
              <div class="input-group shadow-sm">
                <input type="file" name="" id="pic-news" class="form-control rounded-end-1 " ref="file" @change="getFile"
                       accept="image/*">
                <button class="send-image btn btn-sm btn-primary text-nowrap rounded-0 rounded-start-1  input-group-text" @click.prevent="sendFile" > ارسال عکس </button>
              </div>
            </div>
            <div>
              <label class="form-label mt-2" for="subTitle">تگ ها :</label>
              <input type="text" name="" id="subTitle" class="form-control rounded-1 shadow-sm" ref="tagsElement" @keyup.enter.self="pushTags"
                     v-model="tags" autocomplete="off">
              <ul class="tags d-flex gap-2 list-unstyled mt-2">
                <li class="badge rounded-pill text-bg-primary d-flex align-items-center py-1 px-2 gap-2 px-lg-3 py-lg-2" v-for="(tag , index) in fileData.tags" :key="index" @click="deleteTags(index)">
                    <span><i class="fa fa-close fa-sm text-white d-flex align-items-center"></i></span>
                  <span class="   ">
                    {{ tag }}
                  </span>
                </li>
              </ul>
            </div>
            <div class="">
              <label class="form-label mt-2" for="summary">خلاصه توضیحات :</label>
              <textarea name="" id="summary" class="form-control rounded-1 shadow-sm" v-model="fileData.excerpt"></textarea>
            </div>
            <div id="container">
              <label class="form-label mt-2 " for="editor">توضیحات :</label>
              <ckeditor :editor="editor" v-model="fileData.body" class="text-start "/>
              <div class="input-group ">
                <button class="btn w-100 btn-primary shadow-sm mt-4" @keydown.prevent="">ارسال</button>
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
input[type="file"]:focus{
  box-shadow: none !important;
}
#summary {
  height: 100px;
}
</style>