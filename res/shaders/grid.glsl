//shader:vertex
#version 410

layout(location=0) in vec3 vertPosIn;
layout(location=1) in vec4 vertNormalIn;
layout(location=2) in vec2 vertUV0In;
layout(location=3) in vec4 vertColorIn;

out vec2 vertUV0;
out vec4 vertColor;
out vec3 fragPos;

uniform mat4 modelMat;
uniform mat4 projViewMat;

void main()
{
    vertUV0 = vertUV0In;
    vertColor = vertColorIn;
    fragPos = vec3(modelMat * vec4(vertPosIn, 1.0));

    gl_Position = projViewMat * modelMat * vec4(vertPosIn, 1.0);
}

//shader:fragment
#version 410

in vec4 vertColor;
in vec2 vertUV0;
in vec3 fragPos;

out vec4 fragColor;

void main()
{
    fragColor = vec4(1, 1, 1, 1);
} 
